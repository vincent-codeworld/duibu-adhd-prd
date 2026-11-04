import SwiftUI

struct EditTaskMetaView: View {
    @EnvironmentObject private var appModel: AppModel
    @EnvironmentObject private var store: LocalStore
    @Environment(\.dismiss) private var dismiss

    let taskId: UUID

    @State private var title = ""
    @State private var notes = ""
    @State private var dueAt = Date()
    @State private var errorMessage: String?

    private var task: TaskItem? { store.task(id: taskId) }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Title", text: $title)
                        .onChange(of: title) { _, value in
                            if value.count > 40 { title = String(value.prefix(40)) }
                        }
                    TextField("Notes (optional)", text: $notes, axis: .vertical)
                        .lineLimit(3...8)
                        .onChange(of: notes) { _, value in
                            if value.count > 500 { notes = String(value.prefix(500)) }
                        }
                    DatePicker("Due", selection: $dueAt, displayedComponents: [.date, .hourAndMinute])
                }
                if let errorMessage {
                    Section { Text(errorMessage).foregroundStyle(.red) }
                }
            }
            .navigationTitle("Edit Task")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
            .onAppear {
                if let task {
                    title = task.title
                    notes = task.notes
                    dueAt = task.dueAt
                }
            }
        }
    }

    private func save() async {
        guard let task else { return }
        do {
            try await appModel.syncSyncedTaskMetadata(taskId: task.id, title: title, notes: notes, dueAt: dueAt)
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
