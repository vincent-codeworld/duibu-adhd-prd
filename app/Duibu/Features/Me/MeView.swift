import SwiftUI
import AuthenticationServices

struct MeView: View {
    @EnvironmentObject private var store: LocalStore
    @EnvironmentObject private var appModel: AppModel
    @State private var errorMessage: String?
    @State private var isBusy = false
    @State private var unreadCount = 0

    var body: some View {
        NavigationStack {
            List {
                Section {
                    if store.isLoggedIn {
                        Text(store.accountNickname ?? "Signed in")
                        if let invite = store.inviteCode {
                            LabeledContent("Invite code", value: invite)
                        }
                        Button("Refresh profile") {
                            Task { await refresh() }
                        }
                        .disabled(isBusy)
                        Button("Reset invite code") {
                            Task { await resetCode() }
                        }
                        .disabled(isBusy)
                        Button("Sign out", role: .destructive) {
                            Task { await appModel.logout() }
                        }
                        Text("Signing out keeps synced tasks in your account. Device-only leftovers stay on this phone.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    } else {
                        NavigationLink("Sign in with Apple") {
                            LoginPlaceholderView()
                        }
                        .accessibilityIdentifier("me.login")
                        Text("You can use local rewards without signing in. For API smoke tests, use stub login.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } header: {
                    StepBuddySectionHeader(title: "Account", systemImage: "person.crop.circle.fill", tone: .accent)
                }

                Section {
                    if store.isLoggedIn {
                        NavigationLink("Friends & invite code") {
                            FriendsView()
                        }
                    } else {
                        Text("Sign in to invite friends.")
                            .foregroundStyle(.secondary)
                    }
                } header: {
                    StepBuddySectionHeader(title: "Friends", systemImage: "person.2.fill", tone: .secondary)
                }

                Section {
                    if store.isLoggedIn {
                        NavigationLink {
                            NoticesView()
                        } label: {
                            HStack {
                                Text("In-app notices")
                                Spacer()
                                if unreadCount > 0 {
                                    Text(String(unreadCount))
                                        .font(.caption.weight(.semibold))
                                        .foregroundStyle(.white)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 2)
                                        .background(Capsule().fill(StepBuddyTheme.accent))
                                }
                            }
                        }
                        .accessibilityIdentifier("me.notices")
                    } else {
                        Text("Sign in to see verification notices.")
                            .foregroundStyle(.secondary)
                    }
                } header: {
                    StepBuddySectionHeader(title: "Notices", systemImage: "bell.fill", tone: .warning)
                }

                Section {
                    Text("StepBuddy is a planning and agreement notebook — not medical advice or treatment.")
                    Text("Signing in uploads device-only tasks to your account. Until then, uninstalling or losing this phone deletes them.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text("Tasks on this device: \(store.tasks.count)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } header: {
                    StepBuddySectionHeader(title: "About & privacy", systemImage: "heart.text.square.fill", tone: .success)
                }

                if let errorMessage {
                    Section {
                        Text(errorMessage).foregroundStyle(.red)
                    }
                }
            }
            .navigationTitle("Me")
            .stepBuddyListChrome()
            .tint(StepBuddyTheme.accent)
            .task { await refreshNoticesBadge() }
            // Pop back from NoticesView does not re-run `.task`; refresh badge on appear.
            .onAppear { Task { await refreshNoticesBadge() } }
        }
    }

    private func refreshNoticesBadge() async {
        guard store.isLoggedIn else {
            unreadCount = 0
            return
        }
        do {
            await appModel.api.setToken(store.accessToken)
            let res = try await appModel.api.listNotices()
            unreadCount = res.unreadCount
        } catch {
            // Badge is best-effort; keep silent on Me tab.
        }
    }

    private func refresh() async {
        isBusy = true
        defer { isBusy = false }
        do {
            try await appModel.refreshMe()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func resetCode() async {
        isBusy = true
        defer { isBusy = false }
        do {
            try await appModel.resetInviteCode()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

struct LoginPlaceholderView: View {
    @EnvironmentObject private var store: LocalStore
    @EnvironmentObject private var appModel: AppModel
    @Environment(\.dismiss) private var dismiss

    @State private var subject = "dev-jennie"
    @State private var nickname = "Jennie"
    @State private var errorMessage: String?
    @State private var isBusy = false

    var body: some View {
        Form {
            Section("Sign in with Apple") {
                Text("On device use `AUTH_MODE=apple` + `APPLE_CLIENT_ID=com.stepbuddy.app`. Sign-in never uploads device-only tasks.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                TextField("Nickname (optional, max 20)", text: $nickname)
                    .onChange(of: nickname) { _, value in
                        if value.count > 20 { nickname = String(value.prefix(20)) }
                    }
                SignInWithAppleButton(.signIn) { request in
                    request.requestedScopes = [.fullName]
                } onCompletion: { result in
                    Task { await handleApple(result) }
                }
                .signInWithAppleButtonStyle(.black)
                .frame(height: 44)
                .accessibilityIdentifier("login.apple")
                .disabled(isBusy)
            }

            Section("Dev / stub") {
                Text("Simulator / `AUTH_MODE=stub`: keep using `POST /v1/auth/dev`, or send any stable string as identityToken to `POST /v1/auth/apple`.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                TextField("Dev subject", text: $subject)
                    .textInputAutocapitalization(.never)
                Button(isBusy ? "Signing in…" : "Dev login (stub)") {
                    Task { await loginStub() }
                }
                .buttonStyle(.borderedProminent)
                .accessibilityIdentifier("login.stub")
                .disabled(isBusy || subject.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                Button(isBusy ? "Signing in…" : "Stub via /v1/auth/apple") {
                    Task { await loginAppleStub() }
                }
                .accessibilityIdentifier("login.apple.stub")
                .disabled(isBusy || subject.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }

            if let errorMessage {
                Section { Text(errorMessage).foregroundStyle(.red) }
            }
        }
        .navigationTitle("Sign in")
        .disabled(isBusy)
    }

    private func trimmedNickname() -> String? {
        let name = nickname.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty else { return nil }
        return String(name.prefix(20))
    }

    private func handleApple(_ result: Result<ASAuthorization, Error>) async {
        isBusy = true
        defer { isBusy = false }
        do {
            let auth = try result.get()
            guard let credential = auth.credential as? ASAuthorizationAppleIDCredential,
                  let tokenData = credential.identityToken,
                  let identityToken = String(data: tokenData, encoding: .utf8),
                  !identityToken.isEmpty
            else {
                errorMessage = "No Apple identityToken received."
                return
            }
            var nick = trimmedNickname()
            if nick == nil, let full = credential.fullName {
                let joined = PersonNameComponentsFormatter.localizedString(from: full, style: .default)
                let cleaned = joined.trimmingCharacters(in: .whitespacesAndNewlines)
                if !cleaned.isEmpty { nick = String(cleaned.prefix(20)) }
            }
            try await appModel.loginWithApple(identityToken: identityToken, nickname: nick)
            errorMessage = nil
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func loginStub() async {
        let name = nickname.trimmingCharacters(in: .whitespacesAndNewlines)
        let sub = subject.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !sub.isEmpty else { return }
        let nick = (1...20).contains(name.count) ? name : "Jennie"
        isBusy = true
        defer { isBusy = false }
        do {
            try await appModel.loginWithDevStub(subject: sub, nickname: nick)
            errorMessage = nil
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func loginAppleStub() async {
        let sub = subject.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !sub.isEmpty else { return }
        isBusy = true
        defer { isBusy = false }
        do {
            try await appModel.loginWithApple(identityToken: sub, nickname: trimmedNickname())
            errorMessage = nil
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
