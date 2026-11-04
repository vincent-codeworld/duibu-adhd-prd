import SwiftUI

struct EditStagesView: View {
    @EnvironmentObject private var appModel: AppModel
    @EnvironmentObject private var store: LocalStore
    @Environment(\.dismiss) private var dismiss

    let taskId: UUID

    @State private var drafts: [StageDraft] = []
    @State private var errorMessage: String?
    @State private var pendingDelete: StageDraft?
    @State private var showDeleteConfirm = false

    private var task: TaskItem? { store.task(id: taskId) }

    private var filledCount: Int {
        drafts.filter { !$0.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }.count
    }

    private var canSave: Bool {
        (3...7).contains(filledCount)
            && drafts.allSatisfy {
                let t = $0.title.trimmingCharacters(in: .whitespacesAndNewlines)
                return t.isEmpty || (1...40).contains(t.count)
            }
            && drafts.filter {
                !$0.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            }.allSatisfy {
                (1...40).contains($0.title.trimmingCharacters(in: .whitespacesAndNewlines).count)
            }
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("You must save with 3–7 stages. While editing you can go outside that range; Save stays locked until it’s valid.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Section("Stages") {
                    ForEach(Array(drafts.enumerated()), id: \.element.id) { index, _ in
                        stageEditor(index: index)
                    }

                    if drafts.count < 7 {
                        Button("Add step") {
                            drafts.append(StageDraft(title: "New step"))
                        }
                    } else {
                        Text("Max 7 stages. To split further, edit or remove an existing one.")
                            .font(.caption)
                            .foregroundStyle(.orange)
                    }

                    if filledCount < 3 {
                        Text("Keep at least 3 stages — rewrite one into a smaller step if needed.")
                            .font(.caption)
                            .foregroundStyle(.orange)
                    }
                }

                if let errorMessage {
                    Section {
                        Text(errorMessage).foregroundStyle(.red)
                    }
                }
            }
            .navigationTitle("Edit Stages")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(!canSave)
                }
            }
            .onAppear { load() }
            .confirmationDialog(
                deleteConfirmMessage,
                isPresented: $showDeleteConfirm,
                titleVisibility: .visible
            ) {
                Button("Delete this step", role: .destructive) {
                    if let pendingDelete,
                       let index = drafts.firstIndex(where: { $0.id == pendingDelete.id }) {
                        drafts.remove(at: index)
                    }
                    pendingDelete = nil
                }
                Button("Cancel", role: .cancel) {
                    pendingDelete = nil
                }
            }
        }
    }

    @ViewBuilder
    private func stageEditor(index: Int) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            TextField("Step \(index + 1) title", text: bindingTitle(at: index))
                .onChange(of: drafts[index].title) { _, value in
                    if value.count > 40 {
                        drafts[index].title = String(value.prefix(40))
                    }
                }

            Toggle("Stage reminder", isOn: Binding(
                get: { drafts[index].remindAt != nil },
                set: { on in
                    if on {
                        let due = store.task(id: taskId)?.dueAt ?? Date().addingTimeInterval(86400)
                        drafts[index].remindAt = min(due, Date().addingTimeInterval(3600))
                    } else {
                        drafts[index].remindAt = nil
                    }
                }
            ))
            if drafts[index].remindAt != nil {
                DatePicker(
                    "Remind at",
                    selection: Binding(
                        get: { drafts[index].remindAt ?? Date() },
                        set: { drafts[index].remindAt = $0 }
                    ),
                    displayedComponents: [.date, .hourAndMinute]
                )
            }

            HStack(spacing: 12) {
                Button {
                    moveUp(index)
                } label: {
                    Label("Move up", systemImage: "arrow.up")
                }
                .disabled(index == 0)

                Button {
                    moveDown(index)
                } label: {
                    Label("Move down", systemImage: "arrow.down")
                }
                .disabled(index >= drafts.count - 1)

                Spacer()

                Button(role: .destructive) {
                    requestDelete(at: index)
                } label: {
                    Label("Delete", systemImage: "trash")
                }
            }
            .font(.caption)
            .buttonStyle(.borderless)

            if let status = drafts[index].statusLabel {
                Text(status)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 4)
    }

    private var deleteConfirmMessage: String {
        guard let draft = pendingDelete else {
            return "Delete this step?"
        }
        if draft.hadReward || draft.wasChecked {
            return "This stage has a reward or is checked. Deleting can’t be undone. Delete anyway?"
        }
        return "Delete this step?"
    }

    private func bindingTitle(at index: Int) -> Binding<String> {
        Binding(
            get: { drafts[index].title },
            set: { drafts[index].title = $0 }
        )
    }

    private func load() {
        guard let task else { return }
        drafts = task.stages
            .sorted { $0.position < $1.position }
            .map { StageDraft(from: $0) }
    }

    private func moveUp(_ index: Int) {
        guard index > 0 else { return }
        drafts.swapAt(index, index - 1)
    }

    private func moveDown(_ index: Int) {
        guard index < drafts.count - 1 else { return }
        drafts.swapAt(index, index + 1)
    }

    private func requestDelete(at index: Int) {
        let draft = drafts[index]
        if draft.hadReward || draft.wasChecked {
            pendingDelete = draft
            showDeleteConfirm = true
        } else {
            drafts.remove(at: index)
        }
    }

    private func save() async {
        guard let task else { return }
        let cleaned = drafts
            .map { draft -> StageDraft in
                var copy = draft
                copy.title = draft.title.trimmingCharacters(in: .whitespacesAndNewlines)
                return copy
            }
            .filter { !$0.title.isEmpty }

        guard (3...7).contains(cleaned.count) else {
            errorMessage = cleaned.count < 3
                ? "Keep at least 3 stages — rewrite one into a smaller step if needed."
                : "Max 7 stages. To split further, edit or remove an existing one."
            return
        }

        let stages: [StageItem] = cleaned.enumerated().map { index, draft in
            StageItem(
                id: draft.existingId ?? UUID(),
                position: index + 1,
                title: String(draft.title.prefix(40)),
                progress: draft.progress,
                reward: draft.reward,
                remindAt: draft.remindAt,
                doneAt: draft.doneAt,
                skippedAt: draft.skippedAt
            )
        }

        do {
            try await appModel.syncSyncedTaskStages(taskId: task.id, stages: stages, source: nil)
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

private struct StageDraft: Identifiable {
    let id: UUID
    var existingId: UUID?
    var title: String
    var progress: StageProgress
    var reward: RewardItem?
    var remindAt: Date?
    var doneAt: Date?
    var skippedAt: Date?

    var hadReward: Bool { reward?.hasContent == true }
    var wasChecked: Bool { progress == .done }

    var statusLabel: String? {
        switch progress {
        case .done: return "Done"
        case .skipped: return "Skipped"
        case .pending: return hadReward ? "Has reward" : nil
        }
    }

    init(title: String) {
        id = UUID()
        existingId = nil
        self.title = title
        progress = .pending
        reward = nil
        remindAt = nil
        doneAt = nil
        skippedAt = nil
    }

    init(from stage: StageItem) {
        id = stage.id
        existingId = stage.id
        title = stage.title
        progress = stage.progress
        reward = stage.reward
        remindAt = stage.remindAt
        doneAt = stage.doneAt
        skippedAt = stage.skippedAt
    }
}
