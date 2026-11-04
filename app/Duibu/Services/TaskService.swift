import Foundation

enum TaskServiceError: LocalizedError, Equatable {
    case emptyTitle
    case invalidStageCount
    case emptyStageTitle
    case missingDueAt
    case remindAfterDue
    case cannotComplete
    case needsVerification
    case cannotUncheck
    case cannotSkip
    case invalidRewardLink
    case rewardEmptyWhenRequired

    var errorDescription: String? {
        switch self {
        case .emptyTitle: return "Add a title to save."
        case .invalidStageCount: return "Save needs 3–7 stages."
        case .emptyStageTitle: return "Stage title can’t be empty."
        case .missingDueAt: return "Pick a due date and time to save."
        case .remindAfterDue: return "Stage reminders must be on or before the task due time."
        case .cannotComplete: return "Still have open stages, or everything is skipped — can’t complete yet."
        case .needsVerification: return "This task has a verifier — request verification instead of completing yourself."
        case .cannotUncheck: return "This reward is already in redeem flow — can’t uncheck."
        case .cannotSkip: return "Uncheck the stage before skipping it."
        case .invalidRewardLink: return "Links must be http or https."
        case .rewardEmptyWhenRequired: return "If there’s a reward, write at least one character."
        }
    }
}

@MainActor
final class TaskService {
    private let store: LocalStore
    private let reminder: ReminderService

    init(store: LocalStore, reminder: ReminderService) {
        self.store = store
        self.reminder = reminder
    }

    func createTask(
        title: String,
        notes: String,
        stages: [StageItem],
        stageSource: StageSource,
        dueAt: Date,
        verifierAccountId: String?
    ) async throws -> TaskItem {
        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard FieldLimits.taskTitle.contains(trimmedTitle.count) else {
            throw TaskServiceError.emptyTitle
        }
        try validateStages(stages)
        try validateReminders(stages: stages, dueAt: dueAt)

        let now = Date()
        let verifier = verifierAccountId
        let task = TaskItem(
            id: UUID(),
            ownerAccountId: nil,
            installId: store.install.installId,
            syncState: .deviceOnly,
            title: String(trimmedTitle.prefix(40)),
            notes: String(notes.prefix(500)),
            status: .active,
            stageSource: stageSource,
            designatedVerifierAccountId: verifier,
            stages: normalized(stages),
            completionReward: nil,
            dueAt: dueAt,
            reminderFireAt: nil,
            completionGate: verifier == nil ? .none : .open,
            createdAt: now,
            updatedAt: now,
            completedAt: nil,
            uploadConsentAt: nil
        )

        // New tasks stay device-only; upload happens via UploadConsentView or auto-upload after login.
        store.upsert(task)
        await reminder.rescheduleAll(for: task)
        return task
    }

    func updateTaskMetadata(_ task: TaskItem, title: String, notes: String, dueAt: Date? = nil) async throws {
        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard FieldLimits.taskTitle.contains(trimmedTitle.count) else {
            throw TaskServiceError.emptyTitle
        }
        var updated = task
        updated.title = String(trimmedTitle.prefix(40))
        updated.notes = String(notes.prefix(500))
        if let dueAt {
            try validateReminders(stages: updated.stages, dueAt: dueAt)
            updated.dueAt = dueAt
        }
        updated.updatedAt = Date()
        store.upsert(updated)
        await reminder.rescheduleAll(for: updated)
    }

    func replaceStages(on task: TaskItem, stages: [StageItem], source: StageSource?) async throws {
        try validateStages(stages)
        try validateReminders(stages: stages, dueAt: task.dueAt)
        var updated = task
        updated.stages = normalized(stages)
        if let source { updated.stageSource = source }
        updated.updatedAt = Date()
        store.upsert(updated)
        await reminder.rescheduleAll(for: updated)
    }

    func check(stageId: UUID, on task: TaskItem) {
        guard var stage = task.stages.first(where: { $0.id == stageId }),
              stage.progress == .pending else { return }
        stage.progress = .done
        stage.doneAt = Date()
        stage.skippedAt = nil
        mutate(task) { $0.stages = replace($0.stages, with: stage) }
    }

    func uncheck(stageId: UUID, on task: TaskItem) throws {
        guard var stage = task.stages.first(where: { $0.id == stageId }) else { return }
        guard stage.canUncheck else { throw TaskServiceError.cannotUncheck }
        stage.progress = .pending
        stage.doneAt = nil
        mutate(task) { $0.stages = replace($0.stages, with: stage) }
    }

    func skip(stageId: UUID, on task: TaskItem) throws {
        guard var stage = task.stages.first(where: { $0.id == stageId }) else { return }
        guard stage.progress == .pending else { throw TaskServiceError.cannotSkip }
        stage.progress = .skipped
        stage.skippedAt = Date()
        mutate(task) { $0.stages = replace($0.stages, with: stage) }
    }

    func unskip(stageId: UUID, on task: TaskItem) {
        guard var stage = task.stages.first(where: { $0.id == stageId }),
              stage.progress == .skipped else { return }
        stage.progress = .pending
        stage.skippedAt = nil
        mutate(task) { $0.stages = replace($0.stages, with: stage) }
    }

    func complete(_ task: TaskItem) throws {
        if task.hasVerifier && task.completionGate != .approved {
            throw TaskServiceError.needsVerification
        }
        guard task.stagesReady else { throw TaskServiceError.cannotComplete }
        var updated = task
        updated.status = .completed
        updated.completedAt = Date()
        updated.updatedAt = Date()
        updated.reminderFireAt = nil
        store.upsert(updated)
        reminder.cancelAll(for: task)
    }

    func markCompletionVerificationRequested(on task: TaskItem) {
        guard task.stagesReady, task.hasVerifier else { return }
        var updated = task
        updated.completionGate = .pending
        updated.updatedAt = Date()
        store.upsert(updated)
    }

    func markCompletionVerificationApproved(on task: TaskItem) throws {
        guard task.stagesReady, task.hasVerifier else { throw TaskServiceError.cannotComplete }
        var updated = task
        updated.completionGate = .approved
        updated.status = .completed
        updated.completedAt = Date()
        updated.updatedAt = Date()
        store.upsert(updated)
        reminder.cancelAll(for: task)
    }

    func setDesignatedVerifier(on task: TaskItem, accountId: String?) {
        var updated = task
        updated.designatedVerifierAccountId = accountId
        updated.completionGate = accountId == nil ? .none : .open
        updated.updatedAt = Date()
        store.upsert(updated)
    }

    func setStageReward(stageId: UUID, on task: TaskItem, text: String, link: String?) throws {
        let reward = try makeReward(scope: .stage, text: text, link: link, existing: task.stages.first(where: { $0.id == stageId })?.reward)
        mutate(task) { current in
            guard var stage = current.stages.first(where: { $0.id == stageId }) else { return }
            stage.reward = reward
            current.stages = replace(current.stages, with: stage)
        }
    }

    func clearStageReward(stageId: UUID, on task: TaskItem) {
        mutate(task) { current in
            guard var stage = current.stages.first(where: { $0.id == stageId }) else { return }
            stage.reward = nil
            current.stages = replace(current.stages, with: stage)
        }
    }

    func setCompletionReward(on task: TaskItem, text: String, link: String?) throws {
        let reward = try makeReward(scope: .taskCompletion, text: text, link: link, existing: task.completionReward)
        mutate(task) { $0.completionReward = reward }
    }

    func clearCompletionReward(on task: TaskItem) {
        mutate(task) { $0.completionReward = nil }
    }

    func selfRedeemStage(stageId: UUID, on task: TaskItem) {
        mutate(task) { current in
            guard var stage = current.stages.first(where: { $0.id == stageId }),
                  stage.progress == .done,
                  var reward = stage.reward,
                  reward.redeemState == .unrequested || reward.redeemState == .rejected else { return }
            reward.redeemState = .selfRedeemed
            reward.redeemedAt = Date()
            stage.reward = reward
            current.stages = replace(current.stages, with: stage)
        }
    }

    func selfRedeemCompletion(on task: TaskItem) {
        guard task.status == .completed,
              var reward = task.completionReward,
              reward.redeemState == .unrequested || reward.redeemState == .rejected else { return }
        reward.redeemState = .selfRedeemed
        reward.redeemedAt = Date()
        mutate(task) { $0.completionReward = reward }
    }

    func setDueAt(on task: TaskItem, dueAt: Date) async throws {
        try validateReminders(stages: task.stages, dueAt: dueAt)
        var updated = task
        updated.dueAt = dueAt
        updated.updatedAt = Date()
        store.upsert(updated)
        await reminder.rescheduleAll(for: updated)
    }

    /// Legacy name used by older UI — maps to dueAt.
    func setReminder(on task: TaskItem, fireAt: Date?) async {
        guard let fireAt else { return }
        try? await setDueAt(on: task, dueAt: fireAt)
    }

    private func mutate(_ task: TaskItem, _ body: (inout TaskItem) -> Void) {
        var updated = task
        body(&updated)
        updated.updatedAt = Date()
        store.upsert(updated)
    }

    private func validateReminders(stages: [StageItem], dueAt: Date) throws {
        for stage in stages {
            if let remindAt = stage.remindAt, remindAt > dueAt {
                throw TaskServiceError.remindAfterDue
            }
        }
    }

    private func validateStages(_ stages: [StageItem]) throws {
        guard FieldLimits.stageCount.contains(stages.count) else {
            throw TaskServiceError.invalidStageCount
        }
        for stage in stages {
            let title = stage.title.trimmingCharacters(in: .whitespacesAndNewlines)
            guard FieldLimits.stageTitle.contains(title.count) else {
                throw TaskServiceError.emptyStageTitle
            }
        }
    }

    private func normalized(_ stages: [StageItem]) -> [StageItem] {
        stages.enumerated().map { index, stage in
            var copy = stage
            copy.position = index + 1
            copy.title = String(stage.title.trimmingCharacters(in: .whitespacesAndNewlines).prefix(40))
            return copy
        }
    }

    private func replace(_ stages: [StageItem], with stage: StageItem) -> [StageItem] {
        stages.map { $0.id == stage.id ? stage : $0 }
    }


    func applySyncedRemote(_ remote: RemoteTaskView, onto local: TaskItem) {
        var updated = local
        // Keep local UUIDs so navigation / SwiftUI identity stay stable; store server ids separately.
        updated.remoteId = remote.id
        updated.syncState = .synced
        updated.uploadConsentAt = remote.uploadConsentAt
        updated.title = remote.title
        updated.notes = remote.notes
        updated.status = remote.status == "completed" ? .completed : .active
        updated.designatedVerifierAccountId = remote.designatedVerifierAccountId
        updated.dueAt = remote.dueAt
        updated.completedAt = remote.completedAt

        let remoteStages = remote.stages.sorted { $0.position < $1.position }
        var newStages: [StageItem] = []
        for (index, remoteStage) in remoteStages.enumerated() {
            let localStage = updated.stages.first { $0.position == remoteStage.position }
                ?? (index < updated.stages.count ? updated.stages[index] : nil)
            var stage = localStage ?? StageItem(
                id: UUID(),
                remoteId: remoteStage.id,
                position: remoteStage.position,
                title: remoteStage.title,
                progress: StageProgress(rawValue: remoteStage.progress) ?? .pending,
                reward: nil,
                remindAt: nil,
                doneAt: nil,
                skippedAt: nil
            )
            stage.remoteId = remoteStage.id
            stage.position = remoteStage.position
            stage.title = remoteStage.title
            stage.progress = StageProgress(rawValue: remoteStage.progress) ?? .pending
            stage.remindAt = remoteStage.remindAt
            stage.doneAt = remoteStage.progress == "done" ? (stage.doneAt ?? Date()) : nil
            stage.skippedAt = remoteStage.progress == "skipped" ? (stage.skippedAt ?? Date()) : nil
            if let rr = remoteStage.reward {
                var reward = stage.reward ?? RewardItem(
                    id: UUID(),
                    remoteId: rr.id,
                    scope: .stage,
                    text: rr.text,
                    link: rr.link,
                    redeemState: RedeemState(rawValue: rr.redeemState) ?? .unrequested,
                    redeemedAt: nil
                )
                reward.remoteId = rr.id
                reward.text = rr.text
                reward.link = rr.link
                reward.redeemState = RedeemState(rawValue: rr.redeemState) ?? reward.redeemState
                stage.reward = reward
            }
            newStages.append(stage)
        }
        updated.stages = newStages

        if let completion = remote.completionReward {
            var reward = updated.completionReward ?? RewardItem(
                id: UUID(),
                remoteId: completion.id,
                scope: .taskCompletion,
                text: completion.text,
                link: completion.link,
                redeemState: RedeemState(rawValue: completion.redeemState) ?? .unrequested,
                redeemedAt: nil
            )
            reward.remoteId = completion.id
            reward.text = completion.text
            reward.link = completion.link
            reward.redeemState = RedeemState(rawValue: completion.redeemState) ?? reward.redeemState
            updated.completionReward = reward
        }

        if remote.status == "completed" {
            updated.completionGate = remote.designatedVerifierAccountId == nil ? .none : .approved
        } else if remote.designatedVerifierAccountId == nil {
            updated.completionGate = .none
        } else {
            switch RedeemState(rawValue: remote.completionReward?.redeemState ?? "") {
            case .pending:
                updated.completionGate = .pending
            case .approved:
                updated.completionGate = .approved
            case .rejected, .unrequested, .selfRedeemed, .none:
                updated.completionGate = .open
            }
        }

        store.upsert(updated)
        Task { await reminder.rescheduleAll(for: updated) }
    }

    private func camelToSnake(_ value: String) -> String {
        // backend uses unrequested / self_redeemed style already
        value
    }

    private func makeReward(scope: RewardScope, text: String, link: String?, existing: RewardItem?) throws -> RewardItem? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty {
            return nil
        }
        guard trimmed.count <= 80 else {
            throw TaskServiceError.rewardEmptyWhenRequired
        }
        if let link, !link.isEmpty {
            guard let url = URL(string: link), let scheme = url.scheme?.lowercased(),
                  scheme == "http" || scheme == "https", link.count <= 500 else {
                throw TaskServiceError.invalidRewardLink
            }
        }
        return RewardItem(
            id: existing?.id ?? UUID(),
            scope: scope,
            text: trimmed,
            link: link?.isEmpty == true ? nil : link,
            redeemState: existing?.redeemState ?? .unrequested,
            redeemedAt: existing?.redeemedAt
        )
    }
}
