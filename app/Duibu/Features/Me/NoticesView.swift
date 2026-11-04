import SwiftUI

struct NoticesView: View {
    @EnvironmentObject private var store: LocalStore
    @EnvironmentObject private var appModel: AppModel

    @State private var items: [RemoteNotice] = []
    @State private var unreadCount = 0
    @State private var errorMessage: String?
    @State private var isBusy = false

    var body: some View {
        List {
            if !store.isLoggedIn {
                Section {
                    Text("Sign in to see in-app notices.")
                        .foregroundStyle(.secondary)
                    NavigationLink("Sign in") { LoginPlaceholderView() }
                }
            } else {
                Section {
                    if items.isEmpty {
                        Text("No notices yet.")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(items) { notice in
                            Button {
                                Task { await markRead(notice) }
                            } label: {
                                HStack(alignment: .top, spacing: 10) {
                                    Circle()
                                        .fill(notice.isUnread ? Color.accentColor : Color.clear)
                                        .frame(width: 8, height: 8)
                                        .padding(.top, 6)
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(notice.kindLabel)
                                            .font(.body.weight(notice.isUnread ? .semibold : .regular))
                                            .foregroundStyle(.primary)
                                        Text(notice.createdAt.formatted(date: .abbreviated, time: .shortened))
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                        Text("Request " + String(notice.requestId.prefix(8)) + "…")
                                            .font(.caption2)
                                            .foregroundStyle(.tertiary)
                                    }
                                    Spacer(minLength: 0)
                                    if notice.isUnread {
                                        Text("Unread")
                                            .font(.caption2)
                                            .foregroundStyle(.secondary)
                                    }
                                }
                            }
                            .accessibilityIdentifier("notice.row." + notice.id)
                        }
                    }
                } header: {
                    if unreadCount > 0 {
                        Text("Unread " + String(unreadCount))
                    }
                }

                if let errorMessage {
                    Section { Text(errorMessage).foregroundStyle(.red) }
                }
            }
        }
        .navigationTitle("Notices")
        .refreshable { await reload() }
        .task { await reload() }
        .overlay {
            if isBusy { ProgressView().allowsHitTesting(false) }
        }
    }

    private func reload() async {
        guard store.isLoggedIn else {
            items = []
            unreadCount = 0
            return
        }
        isBusy = true
        defer { isBusy = false }
        do {
            await appModel.api.setToken(store.accessToken)
            let res = try await appModel.api.listNotices()
            items = res.items
            unreadCount = res.unreadCount
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func markRead(_ notice: RemoteNotice) async {
        guard notice.isUnread else { return }
        do {
            await appModel.api.setToken(store.accessToken)
            let updated = try await appModel.api.markNoticeRead(id: notice.id)
            if let idx = items.firstIndex(where: { $0.id == notice.id }) {
                items[idx] = updated
            }
            unreadCount = max(0, items.filter { $0.isUnread }.count)
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
