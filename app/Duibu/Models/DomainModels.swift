import Foundation

enum SyncState: String, Codable, Sendable {
    case deviceOnly = "device_only"
    case synced
}

enum TaskStatus: String, Codable, Sendable {
    case active
    case completed
}

enum StageSource: String, Codable, Sendable {
    case manual
    case ai
    case template
}

enum StageProgress: String, Codable, Sendable {
    case pending
    case done
    case skipped
}

enum RewardScope: String, Codable, Sendable {
    case stage
    case taskCompletion = "task_completion"
}

enum RedeemState: String, Codable, Sendable {
    case unrequested
    case pending
    case approved
    case rejected
    case selfRedeemed = "self_redeemed"
}

/// Task-completion verification gate when a verifier is designated (PRD v1.1).
enum CompletionGate: String, Codable, Sendable {
    case none
    case open
    case pending
    case approved
}

struct LocalInstall: Codable, Sendable, Equatable {
    var installId: UUID
    var createdAt: Date
}

struct TaskItem: Identifiable, Codable, Sendable, Equatable {
    var id: UUID
    /// Server task id when synced; API calls prefer this over local `id`.
    var remoteId: String? = nil
    var ownerAccountId: String?
    var installId: UUID
    var syncState: SyncState
    var title: String
    var notes: String
    var status: TaskStatus
    var stageSource: StageSource
    var designatedVerifierAccountId: String?
    var stages: [StageItem]
    var completionReward: RewardItem?
    /// Required task end time (local notification).
    var dueAt: Date
    /// Legacy single reminder; migrated into `dueAt` when decoding older payloads.
    var reminderFireAt: Date? = nil
    /// Local gate for verifier-required completion.
    var completionGate: CompletionGate = .none
    var createdAt: Date
    var updatedAt: Date
    var completedAt: Date?
    var uploadConsentAt: Date?

    var apiId: String { remoteId ?? id.uuidString }

    var checkedCount: Int { stages.filter { $0.progress == .done }.count }
    var skippedCount: Int { stages.filter { $0.progress == .skipped }.count }
    var stageTotal: Int { stages.count }

    var progressLabel: String {
        var label = "\(checkedCount)/\(stageTotal) done"
        if skippedCount > 0 {
            label += " · skipped \(skippedCount)"
        }
        return label
    }

    /// Stages ready: no pending, at least one done.
    var stagesReady: Bool {
        status == .active
            && stages.allSatisfy { $0.progress == .done || $0.progress == .skipped }
            && checkedCount >= 1
    }

    var hasVerifier: Bool { designatedVerifierAccountId != nil }

    /// Self-complete only when no verifier (PRD §7).
    var canComplete: Bool {
        stagesReady && !hasVerifier
    }

    /// Needs task-completion verification when verifier is set (must be synced — request hits server).
    var canRequestCompletionVerification: Bool {
        stagesReady && hasVerifier && syncState == .synced
            && (completionGate == .open || completionGate == .none)
    }

    var nextPendingStageId: UUID? {
        stages.first(where: { $0.progress == .pending })?.id
    }

    enum CodingKeys: String, CodingKey {
        case id, remoteId, ownerAccountId, installId, syncState, title, notes, status
        case stageSource, designatedVerifierAccountId, stages, completionReward
        case dueAt, reminderFireAt, completionGate, createdAt, updatedAt, completedAt, uploadConsentAt
    }

    init(
        id: UUID,
        remoteId: String? = nil,
        ownerAccountId: String?,
        installId: UUID,
        syncState: SyncState,
        title: String,
        notes: String,
        status: TaskStatus,
        stageSource: StageSource,
        designatedVerifierAccountId: String?,
        stages: [StageItem],
        completionReward: RewardItem?,
        dueAt: Date,
        reminderFireAt: Date? = nil,
        completionGate: CompletionGate = .none,
        createdAt: Date,
        updatedAt: Date,
        completedAt: Date?,
        uploadConsentAt: Date?
    ) {
        self.id = id
        self.remoteId = remoteId
        self.ownerAccountId = ownerAccountId
        self.installId = installId
        self.syncState = syncState
        self.title = title
        self.notes = notes
        self.status = status
        self.stageSource = stageSource
        self.designatedVerifierAccountId = designatedVerifierAccountId
        self.stages = stages
        self.completionReward = completionReward
        self.dueAt = dueAt
        self.reminderFireAt = reminderFireAt
        self.completionGate = completionGate
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.completedAt = completedAt
        self.uploadConsentAt = uploadConsentAt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(UUID.self, forKey: .id)
        remoteId = try c.decodeIfPresent(String.self, forKey: .remoteId)
        ownerAccountId = try c.decodeIfPresent(String.self, forKey: .ownerAccountId)
        installId = try c.decode(UUID.self, forKey: .installId)
        syncState = try c.decode(SyncState.self, forKey: .syncState)
        title = try c.decode(String.self, forKey: .title)
        notes = try c.decode(String.self, forKey: .notes)
        status = try c.decode(TaskStatus.self, forKey: .status)
        stageSource = try c.decode(StageSource.self, forKey: .stageSource)
        designatedVerifierAccountId = try c.decodeIfPresent(String.self, forKey: .designatedVerifierAccountId)
        stages = try c.decode([StageItem].self, forKey: .stages)
        completionReward = try c.decodeIfPresent(RewardItem.self, forKey: .completionReward)
        reminderFireAt = try c.decodeIfPresent(Date.self, forKey: .reminderFireAt)
        createdAt = try c.decode(Date.self, forKey: .createdAt)
        updatedAt = try c.decode(Date.self, forKey: .updatedAt)
        completedAt = try c.decodeIfPresent(Date.self, forKey: .completedAt)
        uploadConsentAt = try c.decodeIfPresent(Date.self, forKey: .uploadConsentAt)
        completionGate = try c.decodeIfPresent(CompletionGate.self, forKey: .completionGate)
            ?? (designatedVerifierAccountId == nil ? .none : .open)
        if let due = try c.decodeIfPresent(Date.self, forKey: .dueAt) {
            dueAt = due
        } else if let legacy = reminderFireAt {
            dueAt = legacy
        } else {
            dueAt = createdAt.addingTimeInterval(24 * 3600)
        }
    }
}

struct StageItem: Identifiable, Codable, Sendable, Equatable {
    var id: UUID
    var remoteId: String? = nil
    var position: Int
    var title: String
    var progress: StageProgress
    var reward: RewardItem?
    var remindAt: Date? = nil
    var doneAt: Date?
    var skippedAt: Date?

    var canUncheck: Bool {
        guard progress == .done else { return false }
        guard let reward else { return true }
        return reward.redeemState == .unrequested
    }

    var canRedeem: Bool {
        progress == .done && reward != nil && reward?.redeemState == .unrequested
    }
}

struct RewardItem: Identifiable, Codable, Sendable, Equatable {
    var id: UUID
    var remoteId: String? = nil
    var scope: RewardScope
    var text: String
    var link: String?
    var redeemState: RedeemState
    var redeemedAt: Date?

    var hasContent: Bool { !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
}

enum FieldLimits {
    static let taskTitle = 1...40
    static let taskNotes = 0...500
    static let stageTitle = 1...40
    static let rewardText = 0...80
    static let rewardLink = 0...500
    static let stageCount = 3...7
}

enum StageTemplate {
    static let titles = ["Prepare", "Do a small chunk", "Wrap up"]
    static let hint = "A simple 3-step starter — tweak the wording for this task."

    static func makeStages() -> [StageItem] {
        titles.enumerated().map { index, title in
            StageItem(
                id: UUID(),
                position: index + 1,
                title: title,
                progress: .pending,
                reward: nil,
                remindAt: nil,
                doneAt: nil,
                skippedAt: nil
            )
        }
    }
}
