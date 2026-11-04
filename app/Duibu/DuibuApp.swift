import SwiftUI

@main
struct DuibuApp: App {
    @StateObject private var appModel: AppModel

    init() {
        let store = LocalStore()
        if ProcessInfo.processInfo.arguments.contains("-ui-testing-reset") {
            store.resetForUITesting()
        }
        let reminder = ReminderService()
        let tasks = TaskService(store: store, reminder: reminder)
        let api = APIClient(accessToken: store.accessToken)
        _appModel = StateObject(wrappedValue: AppModel(
            store: store,
            taskService: tasks,
            reminder: reminder,
            api: api,
            ai: APIAIBreakdownService(api: api)
        ))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(appModel.store)
                .environmentObject(appModel)
        }
    }
}

@MainActor
final class AppModel: ObservableObject {
    let store: LocalStore
    let taskService: TaskService
    let reminder: ReminderService
    let ai: AIBreakdownServing
    let api: APIClient

    init(
        store: LocalStore,
        taskService: TaskService,
        reminder: ReminderService,
        api: APIClient,
        ai: AIBreakdownServing? = nil
    ) {
        self.store = store
        self.taskService = taskService
        self.reminder = reminder
        self.api = api
        self.ai = ai ?? APIAIBreakdownService(api: api)
    }

    func loginWithDevStub(subject: String, nickname: String) async throws {
        let session = try await api.loginDev(subject: subject, nickname: nickname)
        store.applySession(
            accountId: session.account.id,
            nickname: session.account.nickname,
            inviteCode: session.account.inviteCode,
            accessToken: session.accessToken,
            expiresAt: session.expiresAt
        )
        await uploadDeviceOnlyTasksAfterLogin()
    }

    func loginWithApple(identityToken: String, nickname: String?) async throws {
        let session = try await api.loginApple(identityToken: identityToken, nickname: nickname)
        store.applySession(
            accountId: session.account.id,
            nickname: session.account.nickname,
            inviteCode: session.account.inviteCode,
            accessToken: session.accessToken,
            expiresAt: session.expiresAt
        )
        await uploadDeviceOnlyTasksAfterLogin()
    }

    /// Create locally (device-only), then best-effort auto-upload when already signed in.
    /// Upload failure leaves the task device-only; create still succeeds.
    func createTask(
        title: String,
        notes: String,
        stages: [StageItem],
        stageSource: StageSource,
        dueAt: Date,
        verifierAccountId: String?
    ) async throws -> TaskItem {
        let task = try await taskService.createTask(
            title: title,
            notes: notes,
            stages: stages,
            stageSource: stageSource,
            dueAt: dueAt,
            verifierAccountId: verifierAccountId
        )
        guard store.isLoggedIn else { return task }
        if let failure = await uploadOneDeviceOnlyTask(task) {
            print("[StepBuddy] Create auto-upload failed (task stays on device): \(failure)")
        }
        return store.task(id: task.id) ?? task
    }

    /// After login, best-effort upload every device-only task (Vincent override of old S19 “never auto-upload”).
    /// Failures are logged/collected and do not fail login — TaskDetail keeps a manual Upload fallback.
    @discardableResult
    func uploadDeviceOnlyTasksAfterLogin() async -> [String] {
        guard store.isLoggedIn else { return [] }
        await api.setToken(store.accessToken)
        var failures: [String] = []
        let pending = store.tasks.filter { $0.syncState == .deviceOnly }
        for task in pending {
            if let msg = await uploadOneDeviceOnlyTask(task) {
                failures.append(msg)
            }
        }
        return failures
    }

    /// Shared path for login batch upload, create-time auto-upload, and UploadConsent semantics.
    /// Returns an error message on failure; nil on success / skip.
    @discardableResult
    func uploadOneDeviceOnlyTask(_ task: TaskItem) async -> String? {
        guard store.isLoggedIn else { return "Not signed in." }
        guard task.syncState == .deviceOnly else { return nil }
        await api.setToken(store.accessToken)
        do {
            var stageRewards: [String: RemoteRewardInput] = [:]
            for stage in task.stages {
                if let reward = stage.reward, reward.hasContent {
                    stageRewards[String(stage.position)] = RemoteRewardInput(text: reward.text, link: reward.link)
                }
            }
            let completion: RemoteRewardInput? = {
                guard let reward = task.completionReward, reward.hasContent else { return nil }
                return RemoteRewardInput(text: reward.text, link: reward.link)
            }()
            var remote = try await api.createSyncedTask(
                CreateSyncedTaskRequest(
                    title: task.title,
                    notes: task.notes,
                    uploadConsent: true,
                    dueAt: task.dueAt,
                    stages: task.stages.map {
                        RemoteStageInput(title: $0.title, progress: $0.progress.rawValue, remindAt: $0.remindAt)
                    },
                    stageRewards: stageRewards.isEmpty ? nil : stageRewards,
                    completionReward: completion,
                    designatedVerifierAccountId: task.designatedVerifierAccountId
                )
            )
            if let verifier = task.designatedVerifierAccountId, remote.designatedVerifierAccountId == nil {
                remote = try await api.setVerifier(taskId: remote.id, accountId: verifier)
            }
            taskService.applySyncedRemote(remote, onto: task)
            return nil
        } catch {
            let msg = "Auto-upload failed for “\(task.title)”: \(error.localizedDescription)"
            print("[StepBuddy] \(msg)")
            return msg
        }
    }

    func refreshMe() async throws {
        let token = store.accessToken
        await api.setToken(token)
        let account = try await api.me()
        store.applySession(
            accountId: account.id,
            nickname: account.nickname,
            inviteCode: account.inviteCode,
            accessToken: token ?? "",
            expiresAt: store.tokenExpiresAt ?? Date().addingTimeInterval(86400)
        )
    }

    func resetInviteCode() async throws {
        await api.setToken(store.accessToken)
        let code = try await api.resetInviteCode()
        store.updateInviteCode(code)
    }

    func logout() async {
        await api.setToken(nil)
        store.clearSession()
    }

    /// Push local stage progress (and optional title/notes) for a synced task.
    func syncSyncedTaskProgress(taskId: UUID) async throws {
        guard let task = store.task(id: taskId), task.syncState == .synced else { return }
        guard store.isLoggedIn else { return }
        await api.setToken(store.accessToken)
        let body = PatchTaskRequest(
            dueAt: task.dueAt,
            stages: task.stages.map {
                RemoteStageInput(title: $0.title, progress: $0.progress.rawValue, remindAt: $0.remindAt)
            }
        )
        let remote = try await api.patchTask(id: task.apiId, body)
        taskService.applySyncedRemote(remote, onto: task)
    }

    func completeSyncedTask(taskId: UUID) async throws {
        guard let task = store.task(id: taskId) else { return }
        if task.syncState == .synced, store.isLoggedIn {
            await api.setToken(store.accessToken)
            // Ensure server stages match before complete
            let body = PatchTaskRequest(
                dueAt: task.dueAt,
                stages: task.stages.map {
                    RemoteStageInput(title: $0.title, progress: $0.progress.rawValue, remindAt: $0.remindAt)
                }
            )
            _ = try await api.patchTask(id: task.apiId, body)
            if task.hasVerifier {
                throw TaskServiceError.needsVerification
            }
            let remote = try await api.completeRemoteTask(id: task.apiId)
            taskService.applySyncedRemote(remote, onto: task)
            if let t = store.task(id: taskId) { reminder.cancelAll(for: t) }
        } else {
            try taskService.complete(task)
        }
    }

    func syncSyncedTaskMetadata(taskId: UUID, title: String, notes: String, dueAt: Date? = nil) async throws {
        guard let existing = store.task(id: taskId) else { return }
        try await taskService.updateTaskMetadata(existing, title: title, notes: notes, dueAt: dueAt)
        guard let task = store.task(id: taskId), task.syncState == .synced, store.isLoggedIn else { return }
        await api.setToken(store.accessToken)
        let body = PatchTaskRequest(title: task.title, notes: task.notes, dueAt: task.dueAt)
        let remote = try await api.patchTask(id: task.apiId, body)
        taskService.applySyncedRemote(remote, onto: task)
    }

    func syncSyncedTaskStages(taskId: UUID, stages: [StageItem], source: StageSource?) async throws {
        guard let task = store.task(id: taskId) else { return }
        try await taskService.replaceStages(on: task, stages: stages, source: source)
        guard let updated = store.task(id: taskId), updated.syncState == .synced, store.isLoggedIn else { return }
        await api.setToken(store.accessToken)
        var stageRewards: [String: RemoteRewardInput] = [:]
        for stage in updated.stages {
            if let reward = stage.reward, reward.hasContent {
                stageRewards[String(stage.position)] = RemoteRewardInput(text: reward.text, link: reward.link)
            }
        }
        let body = PatchTaskRequest(
            dueAt: updated.dueAt,
            stages: updated.stages.map {
                RemoteStageInput(title: $0.title, progress: $0.progress.rawValue, remindAt: $0.remindAt)
            },
            stageRewards: stageRewards.isEmpty ? nil : stageRewards
        )
        let remote = try await api.patchTask(id: updated.apiId, body)
        taskService.applySyncedRemote(remote, onto: updated)
    }
}
