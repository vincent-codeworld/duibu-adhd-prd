import SwiftUI

/// S19: explicit per-task upload consent before synced actions.
struct UploadConsentView: View {
    @EnvironmentObject private var appModel: AppModel
    @EnvironmentObject private var store: LocalStore
    @Environment(\.dismiss) private var dismiss

    let taskId: UUID
    var onUploaded: ((TaskItem) -> Void)?

    @State private var isBusy = false
    @State private var errorMessage: String?
    @State private var selectedVerifierId: String?
    @State private var friends: [RemoteFriend] = []

    private var task: TaskItem? { store.task(id: taskId) }
    private var existingVerifierId: String? { task?.designatedVerifierAccountId }
    private var hasExistingVerifier: Bool { existingVerifierId != nil }

    private var existingVerifierLabel: String {
        guard let id = existingVerifierId else { return "—" }
        return friends.first(where: { $0.accountId == id })?.nickname ?? id
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("Only this task’s title, notes, stages, and reward text upload. Other tasks stay on this phone.")
                        .font(.body)
                }

                if let task {
                    Section("Will upload") {
                        Label(task.title, systemImage: "square.and.arrow.up")
                            .font(.headline)
                        Text("\(task.stages.count) stages")
                            .foregroundStyle(.secondary)
                    }

                    if store.isLoggedIn {
                        if hasExistingVerifier {
                            Section {
                                LabeledContent("Verifier", value: existingVerifierLabel)
                                Text("Chosen when you created this task — no need to pick again.")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            } header: {
                                Label("Verifier", systemImage: "person.badge.shield.checkmark")
                            }
                        } else {
                            Section {
                                if friends.isEmpty {
                                    Text("No friends yet. You can upload now and choose a verifier later.")
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                } else {
                                    Picker("Verifier", selection: $selectedVerifierId) {
                                        Text("None for now").tag(String?.none)
                                        ForEach(friends) { friend in
                                            Text(friend.nickname).tag(Optional(friend.accountId))
                                        }
                                    }
                                    .pickerStyle(.menu)
                                    .accessibilityIdentifier("upload.verifier")
                                }
                            } header: {
                                Label("Choose verifier (optional)", systemImage: "person.badge.shield.checkmark")
                            }
                        }
                    }
                }

                if let errorMessage {
                    Section { Text(errorMessage).foregroundStyle(.red) }
                }
            }
            .navigationTitle("Upload this task")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isBusy ? "Uploading…" : "Agree & upload") {
                        Task { await upload() }
                    }
                    .disabled(isBusy || !store.isLoggedIn)
                    .accessibilityIdentifier("upload.confirm")
                }
            }
            .task { await loadFriends() }
        }
    }

    private func loadFriends() async {
        guard store.isLoggedIn else { return }
        do {
            await appModel.api.setToken(store.accessToken)
            friends = try await appModel.api.listFriends()
        } catch {
            // non-blocking
        }
    }

    private func upload() async {
        guard let task else { return }
        guard store.isLoggedIn else {
            errorMessage = "Sign in first."
            return
        }
        isBusy = true
        defer { isBusy = false }
        do {
            await appModel.api.setToken(store.accessToken)
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
            // Prefer create-time verifier; only fall back to picker when none was set.
            let verifierToSend = existingVerifierId ?? selectedVerifierId
            var remote = try await appModel.api.createSyncedTask(
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
                    designatedVerifierAccountId: verifierToSend
                )
            )
            if let verifier = verifierToSend, remote.designatedVerifierAccountId == nil {
                remote = try await appModel.api.setVerifier(taskId: remote.id, accountId: verifier)
            }
            appModel.taskService.applySyncedRemote(remote, onto: task)
            if let updated = store.task(id: UUID(uuidString: remote.id) ?? task.id) {
                onUploaded?(updated)
            }
            errorMessage = nil
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
