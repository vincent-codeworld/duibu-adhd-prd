import Foundation

@MainActor
final class LocalStore: ObservableObject {
    @Published private(set) var install: LocalInstall
    @Published private(set) var tasks: [TaskItem] = []
    @Published private(set) var hasSeenMedicalDisclaimer = false
    @Published var accountNickname: String?
    @Published var isLoggedIn = false
    @Published var accountId: String?
    @Published var accessToken: String?
    @Published var inviteCode: String?
    @Published var tokenExpiresAt: Date?

    private let fileURL: URL
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    init(fileManager: FileManager = .default) {
        encoder.dateEncodingStrategy = .iso8601
        decoder.dateDecodingStrategy = .iso8601

        let dir = fileManager.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
            .appendingPathComponent("Duibu", isDirectory: true)
        try? fileManager.createDirectory(at: dir, withIntermediateDirectories: true)
        fileURL = dir.appendingPathComponent("store.json")

        if let data = try? Data(contentsOf: fileURL),
           let snapshot = try? decoder.decode(StoreSnapshot.self, from: data) {
            install = snapshot.install
            tasks = snapshot.tasks
            hasSeenMedicalDisclaimer = snapshot.hasSeenMedicalDisclaimer
            accountNickname = snapshot.accountNickname
            isLoggedIn = snapshot.isLoggedIn
            accountId = snapshot.accountId
            accessToken = snapshot.accessToken
            inviteCode = snapshot.inviteCode
            tokenExpiresAt = snapshot.tokenExpiresAt
        } else {
            install = LocalInstall(installId: UUID(), createdAt: Date())
            persist()
        }
    }

    func acknowledgeMedicalDisclaimer() {
        hasSeenMedicalDisclaimer = true
        persist()
    }

    func upsert(_ task: TaskItem) {
        if let index = tasks.firstIndex(where: { $0.id == task.id }) {
            tasks[index] = task
        } else {
            tasks.insert(task, at: 0)
        }
        persist()
    }

    func deleteTask(id: UUID) {
        tasks.removeAll { $0.id == id }
        persist()
    }

    func task(id: UUID) -> TaskItem? {
        tasks.first { $0.id == id }
    }

    func setLoggedIn(_ loggedIn: Bool, nickname: String?) {
        // Session flag only. Does not upload or delete device-only tasks.
        isLoggedIn = loggedIn
        accountNickname = nickname
        if !loggedIn {
            accountId = nil
            accessToken = nil
            inviteCode = nil
            tokenExpiresAt = nil
        }
        persist()
    }

    /// Store the app session restored on the next launch. Does not upload tasks.
    /// Callers that just signed in run best-effort auto-upload separately.
    func applySession(accountId: String, nickname: String, inviteCode: String, accessToken: String, expiresAt: Date) {
        self.isLoggedIn = true
        self.accountId = accountId
        self.accountNickname = nickname
        self.inviteCode = inviteCode
        self.accessToken = accessToken
        self.tokenExpiresAt = expiresAt
        persist()
    }

    func updateInviteCode(_ code: String) {
        inviteCode = code
        persist()
    }

    func clearSession() {
        // Keep all local tasks untouched.
        isLoggedIn = false
        accountNickname = nil
        accountId = nil
        accessToken = nil
        inviteCode = nil
        tokenExpiresAt = nil
        persist()
    }

    /// UI tests only: wipe session + tasks, keep install id, mark disclaimer seen.
    func resetForUITesting() {
        tasks = []
        hasSeenMedicalDisclaimer = true
        clearSession()
    }

    private func persist() {
        let snapshot = StoreSnapshot(
            install: install,
            tasks: tasks,
            hasSeenMedicalDisclaimer: hasSeenMedicalDisclaimer,
            accountNickname: accountNickname,
            isLoggedIn: isLoggedIn,
            accountId: accountId,
            accessToken: accessToken,
            inviteCode: inviteCode,
            tokenExpiresAt: tokenExpiresAt
        )
        guard let data = try? encoder.encode(snapshot) else { return }
        try? data.write(to: fileURL, options: [.atomic])
    }
}

private struct StoreSnapshot: Codable {
    var install: LocalInstall
    var tasks: [TaskItem]
    var hasSeenMedicalDisclaimer: Bool
    var accountNickname: String?
    var isLoggedIn: Bool
    var accountId: String?
    var accessToken: String?
    var inviteCode: String?
    var tokenExpiresAt: Date?
}
