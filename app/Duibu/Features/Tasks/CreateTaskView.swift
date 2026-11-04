import SwiftUI

struct CreateTaskView: View {
    @EnvironmentObject private var appModel: AppModel
    @EnvironmentObject private var store: LocalStore
    @Environment(\.dismiss) private var dismiss

    @State private var title = ""
    @State private var notes = ""
    @State private var stageTitles: [String] = StageTemplate.titles
    @State private var stageReminders: [Date?] = [nil, nil, nil]
    @State private var stageSource: StageSource = .manual
    @State private var showTemplateHint = true
    @State private var isBreakingDown = false
    @State private var dueAt = Calendar.current.date(byAdding: .day, value: 1, to: Date()) ?? Date().addingTimeInterval(86400)
    @State private var selectedVerifierId: String?
    @State private var friends: [RemoteFriend] = []
    @State private var errorMessage: String?
    @State private var createdId: UUID?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Title (required)", text: $title)
                        .onChange(of: title) { _, value in
                            if value.count > 40 { title = String(value.prefix(40)) }
                        }
                    TextField("Notes (optional)", text: $notes, axis: .vertical)
                        .lineLimit(3...6)
                    DatePicker("Due", selection: $dueAt, in: Date()..., displayedComponents: [.date, .hourAndMinute])
                } header: {
                    StepBuddySectionHeader(title: "The task", systemImage: "pencil.and.list.clipboard", tone: .accent)
                }

                Section {
                    Button {
                        Task { await runAI() }
                    } label: {
                        if isBreakingDown {
                            HStack(spacing: 10) {
                                ProgressView()
                                Text("Breaking into steps…")
                            }
                        } else {
                            Label("Break it into steps", systemImage: "sparkles")
                        }
                    }
                    .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isBreakingDown)

                    Text("Only the title and notes are sent for this breakdown.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Section {
                    if showTemplateHint {
                        Text(StageTemplate.hint)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    ForEach(stageTitles.indices, id: \.self) { index in
                        VStack(alignment: .leading, spacing: StepBuddyTheme.rowSpacing) {
                            HStack {
                                TextField("Step \(index + 1)", text: $stageTitles[index])
                                if stageTitles.count > 3 {
                                    Button(role: .destructive) {
                                        stageTitles.remove(at: index)
                                        if index < stageReminders.count { stageReminders.remove(at: index) }
                                    } label: {
                                        Image(systemName: "minus.circle.fill")
                                    }
                                }
                            }
                            Toggle(isOn: Binding(
                                get: { index < stageReminders.count && stageReminders[index] != nil },
                                set: { on in
                                    ensureReminderSlots()
                                    if on {
                                        stageReminders[index] = min(dueAt, Date().addingTimeInterval(3600))
                                    } else {
                                        stageReminders[index] = nil
                                    }
                                }
                            )) {
                                Text("Stage reminder")
                                    .font(.caption)
                            }
                            if index < stageReminders.count, let _ = stageReminders[index] {
                                DatePicker(
                                    "Remind at",
                                    selection: Binding(
                                        get: { stageReminders[index] ?? dueAt },
                                        set: { stageReminders[index] = $0 }
                                    ),
                                    in: ...dueAt,
                                    displayedComponents: [.date, .hourAndMinute]
                                )
                                .labelsHidden()
                            }
                        }
                        .padding(.vertical, 2)
                    }
                    if stageTitles.count < 7 {
                        Button {
                            stageTitles.append("New step")
                            stageReminders.append(nil)
                        } label: {
                            Label("Add step", systemImage: "plus.circle")
                        }
                    }
                    if !(3...7).contains(stageTitles.filter { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }.count) {
                        Text(stageTitles.count < 3
                             ? "Keep at least 3 stages — rewrite one into a smaller step if needed"
                             : "Max 7 stages. To split further, edit or remove an existing one")
                            .font(.caption)
                            .foregroundStyle(.orange)
                    }
                } header: {
                    StepBuddySectionHeader(title: "Stages (3–7)", systemImage: "list.number", tone: .secondary)
                }

                Section {
                    if store.isLoggedIn {
                        if friends.isEmpty {
                            Text("No friends yet — you can add one in Me, or save without a verifier.")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        } else {
                            Picker("Verifier", selection: $selectedVerifierId) {
                                Text("None").tag(String?.none)
                                ForEach(friends) { friend in
                                    Text(friend.nickname).tag(Optional(friend.accountId))
                                }
                            }
                            .pickerStyle(.menu)
                            .accessibilityIdentifier("create.verifier")
                            Text("With a verifier, you must request verification after stages are ready — they approve before the task completes.")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    } else {
                        Text("Sign in to choose a verifier.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } header: {
                    StepBuddySectionHeader(title: "Verifier (optional)", systemImage: "person.badge.shield.checkmark", tone: .secondary)
                } footer: {
                    Text("New tasks save on this phone first. If you’re signed in they’re uploaded right away; otherwise signing in uploads leftovers. Upload on the task is the fallback if auto-upload fails.")
                        .font(.caption)
                }

                if let errorMessage {
                    Section {
                        Text(errorMessage).foregroundStyle(.red)
                    }
                }
            }
            .navigationTitle("New Task")
            .navigationBarTitleDisplayMode(.inline)
            .stepBuddyListChrome()
            .tint(StepBuddyTheme.accent)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .accessibilityIdentifier("create.save")
                        .disabled(!canSave)
                }
            }
            .task { await loadFriends() }
        }
    }

    private var canSave: Bool {
        let t = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let stages = stageTitles.map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty }
        return (1...40).contains(t.count) && (3...7).contains(stages.count)
    }

    private func ensureReminderSlots() {
        while stageReminders.count < stageTitles.count { stageReminders.append(nil) }
        if stageReminders.count > stageTitles.count {
            stageReminders = Array(stageReminders.prefix(stageTitles.count))
        }
    }

    private func loadFriends() async {
        guard store.isLoggedIn else { return }
        do {
            await appModel.api.setToken(store.accessToken)
            friends = try await appModel.api.listFriends()
            if friends.isEmpty {
                errorMessage = "No friends loaded from API."
            }
        } catch {
            friends = []
            errorMessage = "Friends load failed: \(error.localizedDescription)"
        }
    }

    private func runAI() async {
        isBreakingDown = true
        defer { isBreakingDown = false }
        let result = await AIBreakdownGate.resolve(
            title: title,
            notes: notes,
            service: appModel.ai
        )
        stageTitles = result.titles
        stageReminders = Array(repeating: nil, count: result.titles.count)
        stageSource = result.source
        showTemplateHint = (result.source == .template)
    }

    private func save() async {
        do {
            ensureReminderSlots()
            let stages = stageTitles
                .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
                .enumerated()
                .map { index, text in
                    StageItem(
                        id: UUID(),
                        position: index + 1,
                        title: text,
                        progress: .pending,
                        reward: nil,
                        remindAt: index < stageReminders.count ? stageReminders[index] : nil,
                        doneAt: nil,
                        skippedAt: nil
                    )
                }
            // AppModel owns create + best-effort auto-upload when already signed in.
            let task = try await appModel.createTask(
                title: title,
                notes: notes,
                stages: stages,
                stageSource: stageSource,
                dueAt: dueAt,
                verifierAccountId: store.isLoggedIn ? selectedVerifierId : nil
            )
            createdId = task.id
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
