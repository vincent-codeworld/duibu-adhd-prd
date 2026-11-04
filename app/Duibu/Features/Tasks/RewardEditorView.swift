import SwiftUI

struct RewardEditorView: View {
    @Environment(\.dismiss) private var dismiss

    let title: String
    @State private var text: String
    @State private var link: String
    @State private var errorMessage: String?

    let onSave: (String, String?) throws -> Void

    init(title: String, initialText: String, initialLink: String, onSave: @escaping (String, String?) throws -> Void) {
        self.title = title
        _text = State(initialValue: initialText)
        _link = State(initialValue: initialLink)
        self.onSave = onSave
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Reward note (optional)", text: $text, axis: .vertical)
                        .lineLimit(2...4)
                        .onChange(of: text) { _, value in
                            if value.count > 80 { text = String(value.prefix(80)) }
                        }
                    TextField("Optional http(s) link", text: $link)
                        .textInputAutocapitalization(.never)
                        .keyboardType(.URL)
                    Text("This is your agreement record — StepBuddy never handles the reward itself.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let errorMessage {
                    Section { Text(errorMessage).foregroundStyle(.red) }
                }
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        do {
                            try onSave(text, link.isEmpty ? nil : link)
                            dismiss()
                        } catch {
                            errorMessage = error.localizedDescription
                        }
                    }
                }
            }
        }
    }
}
