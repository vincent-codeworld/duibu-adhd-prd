import SwiftUI

struct TaskDetailView: View {
    @EnvironmentObject private var store: LocalStore
    @EnvironmentObject private var appModel: AppModel

    let taskId: UUID

    @State private var activeTaskId: UUID
    @State private var showSkipAlert = false
    @State private var pendingSkipStageId: UUID?
    @State private var editingRewardStage: StageItem?
    @State private var editingCompletionReward = false
    @State private var showSelfRedeemStage: StageItem?
    @State private var showSelfRedeemCompletion = false
    @State private var showReminderPicker = false
    @State private var reminderDraft = Date().addingTimeInterval(3600)
    @State private var errorMessage: String?
    @State private var scrollTarget: UUID?
    @State private var showEditStages = false
    @State private var showEditMeta = false
    @State private var showUploadConsent = false
    @State private var verifyingReward: RewardItem?
    @State private var showPickVerifier = false
    @State private var friends: [RemoteFriend] = []
    @State private var pendingVerifierId: String = ""

    init(taskId: UUID) {
        self.taskId = taskId
        _activeTaskId = State(initialValue: taskId)
    }

    private var task: TaskItem? { store.task(id: activeTaskId) }

    var body: some View {
        Group {
            if let task {
                content(task)
            } else {
                ContentUnavailableView("Task not found", systemImage: "questionmark.folder")
            }
        }
    }

    @ViewBuilder
    private func content(_ task: TaskItem) -> some View {
        ScrollViewReader { proxy in
            List {
                Section {
                    Text(task.title).font(.title2.bold())
                    Label(task.progressLabel, systemImage: "chart.bar.fill")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    if !task.notes.isEmpty {
                        Text(task.notes)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .padding(.top, 2)
                    }
                }

                Section {
                    Label(task.dueAt.formatted(date: .abbreviated, time: .shortened), systemImage: "calendar")
                    Button {
                        reminderDraft = task.dueAt
                        showReminderPicker = true
                    } label: {
                        Label("Change due time", systemImage: "clock")
                    }
                } header: {
                    StepBuddySectionHeader(title: "Due", systemImage: "alarm.fill", tone: .warning)
                }

                Section {

                    ForEach(task.stages) { stage in
                        StageRowView(
                            stage: stage,
                            isSynced: task.syncState == .synced,
                            hasVerifier: task.designatedVerifierAccountId != nil,
                            onToggle: { toggle(stage, on: task) },
                            onSkip: {
                                if ProcessInfo.processInfo.arguments.contains("-ui-testing") {
                                    Task {
                                        do {
                                            try appModel.taskService.skip(stageId: stage.id, on: task)
                                            try await appModel.syncSyncedTaskProgress(taskId: activeTaskId)
                                        } catch {
                                            errorMessage = error.localizedDescription
                                        }
                                    }
                                } else {
                                    pendingSkipStageId = stage.id
                                    showSkipAlert = true
                                }
                            },
                            onUnskip: {
                                Task {
                                    appModel.taskService.unskip(stageId: stage.id, on: task)
                                    do { try await appModel.syncSyncedTaskProgress(taskId: activeTaskId) }
                                    catch { errorMessage = error.localizedDescription }
                                }
                            },
                            onEditReward: { editingRewardStage = stage },
                            onSelfRedeem: { showSelfRedeemStage = stage },
                            onRequestVerification: {
                                if let reward = stage.reward { verifyingReward = reward }
                            }
                        )
                        .id(stage.id)
                        .stepBuddyCardRow(tone: .accent)
                    }
                } header: {
                    StepBuddySectionHeader(title: "Stages", systemImage: "list.number", tone: .accent)
                }

                Section {
                    if task.syncState == .deviceOnly {
                        Text("Still on this phone. Signed-in creates usually upload right away — use Upload below if that didn’t finish.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        if store.isLoggedIn {
                            Button {
                                showUploadConsent = true
                            } label: {
                                Label("Upload this task…", systemImage: "square.and.arrow.up")
                            }
                            .accessibilityIdentifier("task.upload")
                        } else {
                            Text("Sign in to upload (and to choose a verifier).")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if task.designatedVerifierAccountId != nil {
                            Text("Verifier: \(friends.first(where: { $0.accountId == task.designatedVerifierAccountId })?.nickname ?? task.designatedVerifierAccountId ?? "")")
                                .font(.caption)
                        }
                    } else {
                        Label("Synced to account", systemImage: "checkmark.icloud")
                            .font(.caption)
                            .foregroundStyle(StepBuddyTheme.success)
                        if let verifier = task.designatedVerifierAccountId {
                            Text("Verifier: \(friends.first(where: { $0.accountId == verifier })?.nickname ?? verifier)")
                                .font(.caption)
                        } else {
                            Text("No verifier yet")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            if store.isLoggedIn {
                                Button {
                                    Task { await loadFriends(); showPickVerifier = true }
                                } label: {
                                    Label("Choose verifier", systemImage: "person.badge.plus")
                                }
                            } else {
                                Text("Sign in to choose a verifier.")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                } header: {
                    StepBuddySectionHeader(title: "Sync & verification", systemImage: "arrow.triangle.2.circlepath", tone: .secondary)
                }

                Section {

                    if let reward = task.completionReward, reward.hasContent {
                        Text(reward.text)
                        if let link = reward.link { Text(link).font(.caption).foregroundStyle(.blue) }
                        Text(redeemLabel(reward.redeemState)).font(.caption).foregroundStyle(.secondary)
                        if task.status == .completed, reward.redeemState == .unrequested || reward.redeemState == .rejected {
                            if task.syncState == .synced, task.designatedVerifierAccountId != nil {
                                Button("Ask friend to verify completion reward") { verifyingReward = reward }
                            }
                            Button("Self-redeem completion reward") { showSelfRedeemCompletion = true }
                        }
                        Button("Edit completion reward") { editingCompletionReward = true }
                    } else {
                        if task.status != .completed {
                            Text("You can draft the reward before the task is complete.")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        Button("Add completion reward") { editingCompletionReward = true }
                    }
                } header: {
                    StepBuddySectionHeader(title: "Completion reward", systemImage: "gift.fill", tone: .warning)
                }

                Section {
                    if task.status == .active {
                        if task.canComplete {
                            Button("Complete task") {
                                Task {
                                    do { try await appModel.completeSyncedTask(taskId: activeTaskId) }
                                    catch { errorMessage = error.localizedDescription }
                                }
                            }
                            .buttonStyle(.borderedProminent)
                        } else if task.canRequestCompletionVerification {
                            if task.completionGate == .pending {
                                Text("Waiting for verifier…")
                                    .foregroundStyle(.secondary)
                            } else {
                                Button("Request verification") {
                                    Task {
                                        do {
                                            await appModel.api.setToken(store.accessToken)
                                            // Push stages, then refresh so server can auto-create Task completion reward.
                                            try await appModel.syncSyncedTaskProgress(taskId: activeTaskId)
                                            if let refreshed = store.task(id: activeTaskId),
                                               let reward = refreshed.completionReward {
                                                verifyingReward = reward
                                            } else if let reward = task.completionReward {
                                                verifyingReward = reward
                                            } else {
                                                errorMessage = "No completion reward yet — re-upload with a verifier, or add a completion reward."
                                            }
                                        } catch {
                                            errorMessage = error.localizedDescription
                                        }
                                    }
                                }
                                .accessibilityIdentifier("task.requestVerification")
                                .buttonStyle(.borderedProminent)
                                Text("A verifier must approve the task-completion check before this task completes.")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        } else if let next = task.nextPendingStageId {
                            Button("Next") {
                                scrollTarget = next
                                withAnimation { proxy.scrollTo(next, anchor: .center) }
                            }
                        } else {
                            Text("You can’t complete a task if every stage is skipped — mark at least one done.")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    } else {
                        Text("Completed")
                            .foregroundStyle(.secondary)
                    }
                }

                if let errorMessage {
                    Section { Text(errorMessage).foregroundStyle(.red) }
                }
            }
        }
        .navigationTitle("Task")
        .navigationBarTitleDisplayMode(.inline)
        .stepBuddyListChrome()
        .tint(StepBuddyTheme.accent)
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Menu {
                    Button("Edit title & notes") { showEditMeta = true }
                    Button("Edit Stages") { showEditStages = true }
                } label: {
                    Image(systemName: "ellipsis.circle")
                }
                .accessibilityLabel("Edit")
            }
        }
        .alert(
            "Skipping isn’t a failure, and that stage’s reward can’t be redeemed. You can undo skip later.",
            isPresented: $showSkipAlert
        ) {
            Button("Skip for now", role: .destructive) {
                guard let sid = pendingSkipStageId else { return }
                Task {
                    guard let latest = store.task(id: activeTaskId) else { return }
                    do {
                        try appModel.taskService.skip(stageId: sid, on: latest)
                        try await appModel.syncSyncedTaskProgress(taskId: activeTaskId)
                    } catch {
                        errorMessage = error.localizedDescription
                    }
                }
                pendingSkipStageId = nil
            }
            .accessibilityIdentifier("stage.skip.confirm")
            Button("Cancel", role: .cancel) {
                pendingSkipStageId = nil
            }
        }
        .sheet(item: $editingRewardStage) { stage in
            RewardEditorView(
                title: "Stage reward",
                initialText: stage.reward?.text ?? "",
                initialLink: stage.reward?.link ?? ""
            ) { text, link in
                if text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    appModel.taskService.clearStageReward(stageId: stage.id, on: task)
                } else {
                    try appModel.taskService.setStageReward(stageId: stage.id, on: task, text: text, link: link)
                }
                if let latest = store.task(id: activeTaskId) {
                    Task {
                        do {
                            try await appModel.syncSyncedTaskStages(
                                taskId: activeTaskId,
                                stages: latest.stages,
                                source: nil
                            )
                        } catch { errorMessage = error.localizedDescription }
                    }
                }
            }
        }
        .sheet(isPresented: $editingCompletionReward) {
            RewardEditorView(
                title: "Completion reward",
                initialText: task.completionReward?.text ?? "",
                initialLink: task.completionReward?.link ?? ""
            ) { text, link in
                if text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    appModel.taskService.clearCompletionReward(on: task)
                } else {
                    try appModel.taskService.setCompletionReward(on: task, text: text, link: link)
                }
            }
        }
        .confirmationDialog("Mark as self-redeemed?", isPresented: Binding(
            get: { showSelfRedeemStage != nil },
            set: { if !$0 { showSelfRedeemStage = nil } }
        ), titleVisibility: .visible) {
            Button("Confirm self-redeem") {
                if let stage = showSelfRedeemStage {
                    appModel.taskService.selfRedeemStage(stageId: stage.id, on: task)
                }
                showSelfRedeemStage = nil
            }
            Button("Cancel", role: .cancel) { showSelfRedeemStage = nil }
        }
        .confirmationDialog("Mark completion reward as self-redeemed?", isPresented: $showSelfRedeemCompletion, titleVisibility: .visible) {
            Button("Confirm self-redeem") {
                appModel.taskService.selfRedeemCompletion(on: task)
                showSelfRedeemCompletion = false
            }
            Button("Cancel", role: .cancel) { showSelfRedeemCompletion = false }
        }
        .sheet(isPresented: $showReminderPicker) {
            NavigationStack {
                Form {
                    DatePicker("Reminder time", selection: $reminderDraft, in: Date()..., displayedComponents: [.date, .hourAndMinute])
                }
                .navigationTitle("Change due time")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Cancel") { showReminderPicker = false }
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Save") {
                            Task {
                                do {
                                    try await appModel.taskService.setDueAt(on: task, dueAt: reminderDraft)
                                    showReminderPicker = false
                                } catch {
                                    errorMessage = error.localizedDescription
                                    showReminderPicker = false
                                }
                            }
                        }
                    }
                }
            }
            .presentationDetents([.medium])
        }
        .sheet(isPresented: $showEditStages) {
            EditStagesView(taskId: task.id)
        }
        .sheet(isPresented: $showEditMeta) {
            EditTaskMetaView(taskId: task.id)
        }
        .sheet(isPresented: $showUploadConsent) {
            UploadConsentView(taskId: activeTaskId) { updated in
                activeTaskId = updated.id
            }
        }
        .sheet(item: $verifyingReward) { reward in
            RequestVerificationView(
                taskId: activeTaskId,
                rewardId: reward.id,
                rewardTitle: reward.text
            )
        }
        .sheet(isPresented: $showPickVerifier) {
            NavigationStack {
                List {
                    if friends.isEmpty {
                        Text("No friends yet — add one in Me.")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(friends) { friend in
                            Button(friend.nickname) {
                                Task { await setVerifier(friend.accountId, on: task) }
                            }
                            .accessibilityIdentifier("verifier.pick.\(friend.nickname)")
                        }
                    }
                }
                .navigationTitle("Choose verifier")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Cancel") { showPickVerifier = false }
                    }
                }
            }
            .presentationDetents([.medium])
        }
        .task {
            if store.isLoggedIn { await loadFriends() }
        }

    }

    private func loadFriends() async {
        guard store.isLoggedIn else { return }
        do {
            await appModel.api.setToken(store.accessToken)
            friends = try await appModel.api.listFriends()
        } catch {
            // ignore
        }
    }

    private func setVerifier(_ accountId: String, on task: TaskItem) async {
        do {
            await appModel.api.setToken(store.accessToken)
            let remote = try await appModel.api.setVerifier(taskId: task.apiId, accountId: accountId)
            appModel.taskService.applySyncedRemote(remote, onto: task)
            showPickVerifier = false
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func toggle(_ stage: StageItem, on task: TaskItem) {
        Task {
            do {
                switch stage.progress {
                case .pending:
                    appModel.taskService.check(stageId: stage.id, on: task)
                case .done:
                    try appModel.taskService.uncheck(stageId: stage.id, on: task)
                case .skipped:
                    return
                }
                try await appModel.syncSyncedTaskProgress(taskId: activeTaskId)
                errorMessage = nil
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func redeemLabel(_ state: RedeemState) -> String {
        switch state {
        case .unrequested: return "Not requested"
        case .pending: return "Verify"
        case .approved: return "Approved"
        case .rejected: return "Rejected"
        case .selfRedeemed: return "Self-redeemed"
        }
    }
}

struct StageRowView: View {
    let stage: StageItem
    var isSynced: Bool = false
    var hasVerifier: Bool = false
    var onToggle: () -> Void
    var onSkip: () -> Void
    var onUnskip: () -> Void
    var onEditReward: () -> Void
    var onSelfRedeem: () -> Void
    var onRequestVerification: () -> Void = {}

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top, spacing: 12) {
                Button(action: onToggle) {
                    Image(systemName: checkSymbol)
                        .font(.title2)
                        .foregroundStyle(checkColor)
                        .accessibilityLabel(accessibilityCheckName)
                        .accessibilityIdentifier("stage.check.\(stage.position)")
                }
                .buttonStyle(.plain)
                .disabled(stage.progress == .skipped)

                VStack(alignment: .leading, spacing: 4) {
                    Text(stage.title)
                        .strikethrough(stage.progress == .skipped)
                    if let reward = stage.reward, reward.hasContent {
                        Text(reward.text)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Text(redeemLabel(reward.redeemState))
                            .font(.caption2)
                            .foregroundStyle(.tertiary)
                    } else {
                        Text("No reward")
                            .font(.caption2)
                            .foregroundStyle(.tertiary)
                    }
                }
                Spacer()
            }

            HStack {
                Button("Reward") { onEditReward() }
                    .font(.caption)
                if stage.progress == .pending {
                    Button("Skip") { onSkip() }
                        .font(.caption)
                        .accessibilityIdentifier("stage.skip.\(stage.position)")
                }
                if stage.progress == .skipped {
                    Button("Undo skip") { onUnskip() }
                        .font(.caption)
                }
                if stage.canRedeem || (stage.progress == .done && stage.reward?.redeemState == .rejected) {
                    if isSynced, hasVerifier, let reward = stage.reward, reward.redeemState == .unrequested || reward.redeemState == .rejected {
                        Button("Ask to verify") { onRequestVerification() }
                            .font(.caption)
                    }
                    if stage.canRedeem || stage.reward?.redeemState == .rejected {
                        Button("Self-redeem") { onSelfRedeem() }
                            .font(.caption)
                    }
                }
            }
        }
        .padding(.vertical, 4)
    }

    private var checkSymbol: String {
        switch stage.progress {
        case .done: return "checkmark.circle.fill"
        case .skipped: return "forward.circle"
        case .pending: return "circle"
        }
    }

    private var checkColor: Color {
        switch stage.progress {
        case .done: return StepBuddyTheme.success
        case .skipped: return .secondary
        case .pending: return StepBuddyTheme.accent
        }
    }

    private var accessibilityCheckName: String {
        "\(stage.title), \(stage.progress == .done ? "Done" : stage.progress == .skipped ? "Skipped" : "Pending")"
    }

    private func redeemLabel(_ state: RedeemState) -> String {
        switch state {
        case .unrequested: return "Not requested"
        case .pending: return "Verify"
        case .approved: return "Approved"
        case .rejected: return "Rejected"
        case .selfRedeemed: return "Self-redeemed"
        }
    }
}
