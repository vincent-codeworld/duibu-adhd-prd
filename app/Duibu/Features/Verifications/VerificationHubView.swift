import SwiftUI

struct VerificationHubView: View {
    @EnvironmentObject private var store: LocalStore
    @EnvironmentObject private var appModel: AppModel

    @State private var inbox: [VerificationInboxItem] = []
    @State private var outbox: [VerificationOutboxItem] = []
    @State private var errorMessage: String?
    @State private var rejectNote = ""
    @State private var rejectingId: String?
    @State private var reportingTarget: ReportTarget?

    var body: some View {
        NavigationStack {
            Group {
                if store.isLoggedIn {
                    List {
                        Section {
                            if inbox.isEmpty {
                                Text("Nothing waiting for you.")
                                    .foregroundStyle(.secondary)
                                    .stepBuddyCardRow(tone: .secondary)
                            } else {
                                ForEach(inbox) { item in
                                    VStack(alignment: .leading, spacing: 6) {
                                        Text("\(item.applicantNickname) · \(item.taskTitle)")
                                            .font(.headline)
                                        if let stage = item.stageTitle {
                                            Text("Stage: \(stage)").font(.subheadline)
                                        } else {
                                            Text("Whole task complete").font(.subheadline)
                                        }
                                        Text("Reward: \(item.rewardText)")
                                        if !item.note.isEmpty {
                                            Text("Note: \(item.note)").font(.caption)
                                        }
                                        if let key = item.screenshotKey, !key.isEmpty {
                                            InboxScreenshotView(screenshotKey: key)
                                        }
                                        HStack {
                                            Button("Approve") {
                                                Task { await approve(item.requestId) }
                                            }
                                            .buttonStyle(.borderedProminent)
                                            Button("Reject", role: .destructive) {
                                                rejectingId = item.requestId
                                            }
                                            .buttonStyle(.bordered)
                                            Button("Report") {
                                                reportingTarget = ReportTarget(id: item.requestId, title: item.taskTitle)
                                            }
                                            .font(.caption)
                                        }
                                    }
                                    .padding(.vertical, 4)
                                    .stepBuddyCardRow(tone: .accent)
                                }
                            }
                        } header: {
                            StepBuddySectionHeader(title: "Inbox", systemImage: "tray.fill", tone: .accent)
                        }

                        Section {
                            if outbox.isEmpty {
                                Text("No outgoing requests yet.")
                                    .foregroundStyle(.secondary)
                                    .stepBuddyCardRow(tone: .secondary)
                            } else {
                                ForEach(outbox) { item in
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(item.taskTitle).font(.headline)
                                        Text(statusLabel(item.status))
                                            .font(.subheadline)
                                            .foregroundStyle(.secondary)
                                        if let note = item.resolutionNote, !note.isEmpty {
                                            Text("Note: \(note)").font(.caption)
                                        }
                                        HStack {
                                            if item.status == "pending" {
                                                Button("Withdraw") {
                                                    Task { await withdraw(item.requestId) }
                                                }
                                                .font(.caption)
                                            }
                                            Button("Report") {
                                                reportingTarget = ReportTarget(id: item.requestId, title: item.taskTitle)
                                            }
                                            .font(.caption)
                                        }
                                    }
                                    .stepBuddyCardRow(tone: .secondary)
                                }
                            }
                        } header: {
                            StepBuddySectionHeader(title: "Sent", systemImage: "paperplane.fill", tone: .secondary)
                        }

                        if let errorMessage {
                            Section { Text(errorMessage).foregroundStyle(.red) }
                        }
                    }
                    .refreshable { await reload() }
                } else {
                    List {
                        Section {
                            Text("Sign in to ask friends to verify.")
                                .foregroundStyle(.secondary)
                            NavigationLink("Sign in") {
                                LoginPlaceholderView()
                            }
                        } header: {
                            StepBuddySectionHeader(title: "Verify", systemImage: "person.2.fill", tone: .secondary)
                        }
                    }
                }
            }
            .navigationTitle("Verify")
            .stepBuddyListChrome()
            .tint(StepBuddyTheme.accent)
            .task { await reload() }
            .alert("Rejection note (optional)", isPresented: Binding(
                get: { rejectingId != nil },
                set: { if !$0 { rejectingId = nil } }
            )) {
                TextField("Note", text: $rejectNote)
                Button("Confirm reject", role: .destructive) {
                    if let id = rejectingId {
                        Task { await reject(id) }
                    }
                }
                Button("Cancel", role: .cancel) { rejectingId = nil }
            } message: {
                Text("Rejecting does not revert their stage to pending.")
            }
            .sheet(item: $reportingTarget) { target in
                ReportVerificationView(requestId: target.id, contextTitle: target.title)
            }
        }
    }


    private func reload() async {
        guard store.isLoggedIn else { return }
        do {
            await appModel.api.setToken(store.accessToken)
            async let inItems = appModel.api.verificationInbox()
            async let outItems = appModel.api.verificationOutbox()
            inbox = try await inItems
            outbox = try await outItems
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func approve(_ id: String) async {
        do {
            await appModel.api.setToken(store.accessToken)
            try await appModel.api.approveVerification(requestId: id)
            await reload()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func reject(_ id: String) async {
        do {
            await appModel.api.setToken(store.accessToken)
            try await appModel.api.rejectVerification(
                requestId: id,
                resolutionNote: rejectNote.isEmpty ? nil : String(rejectNote.prefix(80))
            )
            rejectNote = ""
            rejectingId = nil
            await reload()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func withdraw(_ id: String) async {
        do {
            await appModel.api.setToken(store.accessToken)
            try await appModel.api.withdrawVerification(requestId: id)
            await reload()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func statusLabel(_ status: String) -> String {
        switch status {
        case "pending": return "Verify"
        case "approved": return "Approved"
        case "rejected": return "Rejected"
        case "withdrawn": return "Withdrawn"
        default: return status
        }
    }
}

private struct ReportTarget: Identifiable {
    let id: String
    let title: String
}

private struct InboxScreenshotView: View {
    @EnvironmentObject private var appModel: AppModel
    @EnvironmentObject private var store: LocalStore
    let screenshotKey: String
    @State private var image: UIImage?
    @State private var failed = false

    var body: some View {
        Group {
            if let image {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFit()
                    .frame(maxHeight: 160)
                    .clipShape(RoundedRectangle(cornerRadius: 8))
            } else if failed {
                Text("Couldn’t load screenshot")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            } else {
                ProgressView()
                    .frame(height: 40)
            }
        }
        .task {
            guard image == nil, !failed else { return }
            do {
                await appModel.api.setToken(store.accessToken)
                let data = try await appModel.api.downloadScreenshot(key: screenshotKey)
                image = UIImage(data: data)
                if image == nil { failed = true }
            } catch {
                failed = true
            }
        }
    }
}
