import SwiftUI

struct ReportVerificationView: View {
    @EnvironmentObject private var store: LocalStore
    @EnvironmentObject private var appModel: AppModel
    @Environment(\.dismiss) private var dismiss

    let requestId: String
    let contextTitle: String

    @State private var reason = "harassment"
    @State private var detail = ""
    @State private var isBusy = false
    @State private var errorMessage: String?
    @State private var receiptMessage: String?

    private let reasons: [(id: String, label: String)] = [
        ("harassment", "Harassment"),
        ("privacy", "Privacy"),
        ("illegal_content", "Illegal content"),
        ("other", "Other"),
    ]

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(contextTitle)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    Text("Reporting never changes the other person’s stage progress or redeem state.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Section("Reason") {
                    Picker("Reason", selection: $reason) {
                        ForEach(reasons, id: \.id) { item in
                            Text(item.label).tag(item.id)
                        }
                    }
                    .pickerStyle(.inline)
                    .labelsHidden()
                }

                Section("Details (optional)") {
                    TextField("Max 200 characters", text: $detail, axis: .vertical)
                        .lineLimit(2...5)
                        .onChange(of: detail) { _, value in
                            if value.count > 200 { detail = String(value.prefix(200)) }
                        }
                }

                if let receiptMessage {
                    Section {
                        Text(receiptMessage)
                            .foregroundStyle(.green)
                    }
                }

                if let errorMessage {
                    Section {
                        Text(errorMessage).foregroundStyle(.red)
                    }
                }
            }
            .navigationTitle("Report")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Submit") { Task { await submit() } }
                        .disabled(isBusy || receiptMessage != nil)
                }
            }
            .overlay {
                if isBusy { ProgressView() }
            }
        }
    }

    private func submit() async {
        isBusy = true
        defer { isBusy = false }
        do {
            await appModel.api.setToken(store.accessToken)
            let trimmed = detail.trimmingCharacters(in: .whitespacesAndNewlines)
            let receipt = try await appModel.api.createReport(
                requestId: requestId,
                reason: reason,
                detail: trimmed.isEmpty ? nil : trimmed
            )
            receiptMessage = receipt.message.isEmpty ? "Received" : receipt.message
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
