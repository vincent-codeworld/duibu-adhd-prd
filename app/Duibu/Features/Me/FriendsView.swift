import SwiftUI

struct FriendsView: View {
    @EnvironmentObject private var appModel: AppModel
    @EnvironmentObject private var store: LocalStore

    @State private var friends: [RemoteFriend] = []
    @State private var inviteInput = ""
    @State private var errorMessage: String?
    @State private var infoMessage: String?
    @State private var isBusy = false

    var body: some View {
        List {
            Section("My invite code") {
                if let code = store.inviteCode {
                    Text(code)
                        .font(.title3.monospaced())
                    Text("Share this code. After they sign in, they enter it to add you.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Text("No invite code yet — refresh your profile.")
                        .foregroundStyle(.secondary)
                }
            }

            Section("Add a friend with their code") {
                TextField("Their invite code", text: $inviteInput)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                Button("Add friend") {
                    Task { await addFriend() }
                }
                .disabled(isBusy || inviteInput.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }

            Section("Friends") {
                if friends.isEmpty {
                    Text("No friends yet.")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(friends) { friend in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(friend.nickname).font(.headline)
                            Text(friend.accountId)
                                .font(.caption2)
                                .foregroundStyle(.tertiary)
                        }
                        .swipeActions {
                            Button("Remove", role: .destructive) {
                                Task { await remove(friend) }
                            }
                            Button("Block") {
                                Task { await block(friend) }
                            }
                            .tint(.orange)
                        }
                    }
                }
            }

            if let infoMessage {
                Section { Text(infoMessage).foregroundStyle(.secondary) }
            }
            if let errorMessage {
                Section { Text(errorMessage).foregroundStyle(.red) }
            }
        }
        .navigationTitle("Friends")
        .refreshable { await load() }
        .task { await load() }
    }

    private func ensureToken() async {
        await appModel.api.setToken(store.accessToken)
    }

    private func load() async {
        guard store.isLoggedIn else { return }
        isBusy = true
        defer { isBusy = false }
        do {
            await ensureToken()
            friends = try await appModel.api.listFriends()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func addFriend() async {
        isBusy = true
        defer { isBusy = false }
        do {
            await ensureToken()
            let result = try await appModel.api.addFriend(
                inviteCode: inviteInput.trimmingCharacters(in: .whitespacesAndNewlines)
            )
            inviteInput = ""
            infoMessage = result.alreadyFriends ? "Already friends." : "Added \(result.friend.nickname)."
            friends = try await appModel.api.listFriends()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func remove(_ friend: RemoteFriend) async {
        do {
            await ensureToken()
            try await appModel.api.removeFriend(friendshipId: friend.friendshipId)
            friends = try await appModel.api.listFriends()
            infoMessage = "Removed \(friend.nickname)."
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func block(_ friend: RemoteFriend) async {
        do {
            await ensureToken()
            try await appModel.api.block(accountId: friend.accountId)
            friends = try await appModel.api.listFriends()
            infoMessage = "Blocked \(friend.nickname)."
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
