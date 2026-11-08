import Foundation

enum APIError: LocalizedError {
    case invalidURL
    case http(Int, String?)
    case decoding
    case unauthorized
    case notImplemented(String?)

    var errorDescription: String? {
        switch self {
        case .invalidURL: return "Invalid API URL."
        case .http(let code, let body): return "Request failed (\(code))\(body.map { ": \($0)" } ?? "")"
        case .decoding: return "Couldn’t parse the server response."
        case .unauthorized: return "Session expired — please sign in again."
        case .notImplemented(let hint): return hint ?? "API not implemented yet."
        }
    }
}

struct AuthSessionResponse: Decodable {
    let accessToken: String
    let expiresAt: Date
    let account: RemoteAccount
    let created: Bool
}

struct RemoteAccount: Decodable, Equatable {
    let id: String
    let nickname: String
    let inviteCode: String
    let createdAt: Date
}

struct InviteCodeResponse: Decodable {
    let inviteCode: String
}

struct RemoteFriend: Decodable, Identifiable, Equatable {
    var id: String { friendshipId }
    let friendshipId: String
    let accountId: String
    let nickname: String
    let since: Date
}

struct FriendsListResponse: Decodable {
    let friends: [RemoteFriend]
}

struct AddFriendResponse: Decodable {
    let friend: RemoteFriend
    let alreadyFriends: Bool
}

actor APIClient {
    var accessToken: String?
    private let baseURL: URL
    private let decoder: JSONDecoder
    private let encoder: JSONEncoder
    private let session: URLSession

    init(baseURL: URL = APIConfig.baseURL, accessToken: String? = nil) {
        self.baseURL = baseURL
        self.accessToken = accessToken
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let raw = try container.decode(String.self)
            let withFraction = ISO8601DateFormatter()
            withFraction.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            let plain = ISO8601DateFormatter()
            plain.formatOptions = [.withInternetDateTime]
            if let date = withFraction.date(from: raw) ?? plain.date(from: raw) {
                return date
            }
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "bad date \(raw)")
        }
        self.decoder = decoder
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        self.encoder = encoder
        self.session = .shared
    }

    func setToken(_ token: String?) {
        accessToken = token
    }

    func loginDev(subject: String, nickname: String?) async throws -> AuthSessionResponse {
        struct Body: Encodable {
            let subject: String
            let nickname: String?
        }
        let response: AuthSessionResponse = try await post(
            path: AuthExchangePath.dev,
            body: Body(subject: subject, nickname: nickname),
            authorized: false
        )
        accessToken = response.accessToken
        return response
    }

    func loginApple(identityToken: String, nickname: String?) async throws -> AuthSessionResponse {
        try await loginWithIdentityToken(
            path: AuthExchangePath.apple,
            identityToken: identityToken,
            nickname: nickname
        )
    }

    /// `POST /v1/auth/google`. Same `AuthResponse` as Apple.
    /// Sends `identityToken` and optional `nickname` only. Does not send email.
    ///
    /// Assumption until the backend route is merged: the server keys the account by the Google `sub`
    /// inside the identity token. With no `GIDServerClientID`, the token `aud` is the iOS client ID.
    /// When `GIDServerClientID` is set, Google sets `aud` to that server client ID.
    func loginGoogle(identityToken: String, nickname: String?) async throws -> AuthSessionResponse {
        try await loginWithIdentityToken(
            path: AuthExchangePath.google,
            identityToken: identityToken,
            nickname: nickname
        )
    }

    private func loginWithIdentityToken(
        path: String,
        identityToken: String,
        nickname: String?
    ) async throws -> AuthSessionResponse {
        let response: AuthSessionResponse = try await post(
            path: path,
            body: IdentityTokenLoginBody(identityToken: identityToken, nickname: nickname),
            authorized: false
        )
        accessToken = response.accessToken
        return response
    }

    func me() async throws -> RemoteAccount {
        try await get(path: "/v1/me", authorized: true)
    }

    func patchNickname(_ nickname: String) async throws -> RemoteAccount {
        struct Body: Encodable { let nickname: String }
        return try await patch(path: "/v1/me", body: Body(nickname: nickname), authorized: true)
    }

    func inviteCode() async throws -> String {
        let res: InviteCodeResponse = try await get(path: "/v1/me/invite-code", authorized: true)
        return res.inviteCode
    }

    func resetInviteCode() async throws -> String {
        let res: InviteCodeResponse = try await postEmpty(path: "/v1/me/invite-code/reset", authorized: true)
        return res.inviteCode
    }

    func listFriends() async throws -> [RemoteFriend] {
        let res: FriendsListResponse = try await get(path: "/v1/friends", authorized: true)
        return res.friends
    }

    func addFriend(inviteCode: String) async throws -> AddFriendResponse {
        struct Body: Encodable { let inviteCode: String }
        return try await post(path: "/v1/friends", body: Body(inviteCode: inviteCode), authorized: true)
    }

    func removeFriend(friendshipId: String) async throws {
        try await deleteNoContent(path: "/v1/friends/\(friendshipId)", authorized: true)
    }

    func block(accountId: String) async throws {
        struct Body: Encodable { let accountId: String }
        try await postNoContent(path: "/v1/blocks", body: Body(accountId: accountId), authorized: true)
    }


    // MARK: - Tasks

    func listTasks() async throws -> [RemoteTaskView] {
        let res: RemoteTasksList = try await get(path: "/v1/tasks", authorized: true)
        return res.tasks
    }

    func createSyncedTask(_ body: CreateSyncedTaskRequest) async throws -> RemoteTaskView {
        try await post(path: "/v1/tasks", body: body, authorized: true)
    }

    func getTask(id: String) async throws -> RemoteTaskView {
        try await get(path: "/v1/tasks/\(id)", authorized: true)
    }

    func deleteTask(id: String) async throws {
        try await deleteNoContent(path: "/v1/tasks/\(id)", authorized: true)
    }

    func completeRemoteTask(id: String) async throws -> RemoteTaskView {
        try await postEmpty(path: "/v1/tasks/\(id)/complete", authorized: true)
    }

    func setVerifier(taskId: String, accountId: String) async throws -> RemoteTaskView {
        try await put(path: "/v1/tasks/\(taskId)/verifier", body: SetVerifierRequest(accountId: accountId), authorized: true)
    }

    func patchTask(id: String, _ body: PatchTaskRequest) async throws -> RemoteTaskView {
        try await patch(path: "/v1/tasks/\(id)", body: body, authorized: true)
    }


    // MARK: - Screenshots (OpenAPI v0.7.0)

    func uploadScreenshot(jpegData: Data) async throws -> ScreenshotUploadResponse {
        try await postMultipart(
            path: "/v1/uploads/screenshots",
            fieldName: "file",
            filename: "screenshot.jpg",
            mimeType: "image/jpeg",
            fileData: jpegData,
            authorized: true
        )
    }

    func downloadScreenshot(key: String) async throws -> Data {
        let request = try makeRequest(path: "/v1/screenshots/\(key)", method: "GET", authorized: true)
        let (data, response) = try await session.data(for: request)
        try validate(response: response, data: data)
        return data
    }

    // MARK: - Verifications

    func submitVerification(rewardId: String, note: String?, screenshotKey: String?) async throws -> VerificationInboxItem {
        try await post(
            path: "/v1/rewards/\(rewardId)/verifications",
            body: SubmitVerificationRequest(note: note, screenshotKey: screenshotKey),
            authorized: true
        )
    }

    func verificationInbox() async throws -> [VerificationInboxItem] {
        let res: VerificationListResponse<VerificationInboxItem> = try await get(path: "/v1/verifications/inbox", authorized: true)
        return res.items
    }

    func verificationOutbox() async throws -> [VerificationOutboxItem] {
        let res: VerificationListResponse<VerificationOutboxItem> = try await get(path: "/v1/verifications/outbox", authorized: true)
        return res.items
    }

    func approveVerification(requestId: String) async throws {
        struct OK: Decodable { let status: String }
        let _: OK = try await postEmpty(path: "/v1/verifications/\(requestId)/approve", authorized: true)
    }

    func rejectVerification(requestId: String, resolutionNote: String?) async throws {
        struct Body: Encodable { let resolutionNote: String? }
        struct OK: Decodable { let status: String }
        let _: OK = try await post(
            path: "/v1/verifications/\(requestId)/reject",
            body: Body(resolutionNote: resolutionNote),
            authorized: true
        )
    }

    func withdrawVerification(requestId: String) async throws {
        struct OK: Decodable { let status: String }
        let _: OK = try await postEmpty(path: "/v1/verifications/\(requestId)/withdraw", authorized: true)
    }

    // MARK: - Reports

    func createReport(requestId: String, reason: String, detail: String?) async throws -> ReportReceipt {
        try await post(
            path: "/v1/reports",
            body: CreateReportRequest(requestId: requestId, reason: reason, detail: detail),
            authorized: true
        )
    }

    // MARK: - AI decompose

    /// No auth (device-only create can call this). Only title/notes allowed.
    func decompose(title: String, notes: String?) async throws -> DecomposeResponse {
        try await post(
            path: "/v1/ai/decompose",
            body: DecomposeRequest(title: title, notes: notes),
            authorized: false
        )
    }

    // MARK: - Notices

    func listNotices() async throws -> NoticesListResponse {
        try await get(path: "/v1/notices", authorized: true)
    }

    func markNoticeRead(id: String) async throws -> RemoteNotice {
        try await postEmpty(path: "/v1/notices/\(id)/read", authorized: true)
    }

    // MARK: - HTTP

    private func get<T: Decodable>(path: String, authorized: Bool) async throws -> T {
        let request = try makeRequest(path: path, method: "GET", authorized: authorized)
        return try await send(request)
    }

    private func post<Body: Encodable, T: Decodable>(path: String, body: Body, authorized: Bool) async throws -> T {
        var request = try makeRequest(path: path, method: "POST", authorized: authorized)
        request.httpBody = try encoder.encode(body)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        return try await send(request)
    }

    private func postEmpty<T: Decodable>(path: String, authorized: Bool) async throws -> T {
        let request = try makeRequest(path: path, method: "POST", authorized: authorized)
        return try await send(request)
    }

    private func postNoContent<Body: Encodable>(path: String, body: Body, authorized: Bool) async throws {
        var request = try makeRequest(path: path, method: "POST", authorized: authorized)
        request.httpBody = try encoder.encode(body)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        try await sendNoContent(request)
    }

    private func patch<Body: Encodable, T: Decodable>(path: String, body: Body, authorized: Bool) async throws -> T {
        var request = try makeRequest(path: path, method: "PATCH", authorized: authorized)
        request.httpBody = try encoder.encode(body)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        return try await send(request)
    }

    private func put<Body: Encodable, T: Decodable>(path: String, body: Body, authorized: Bool) async throws -> T {
        var request = try makeRequest(path: path, method: "PUT", authorized: authorized)
        request.httpBody = try encoder.encode(body)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        return try await send(request)
    }

    private func deleteNoContent(path: String, authorized: Bool) async throws {
        let request = try makeRequest(path: path, method: "DELETE", authorized: authorized)
        try await sendNoContent(request)
    }

    private func postMultipart<T: Decodable>(
        path: String,
        fieldName: String,
        filename: String,
        mimeType: String,
        fileData: Data,
        authorized: Bool
    ) async throws -> T {
        let boundary = "Boundary-\(UUID().uuidString)"
        var request = try makeRequest(path: path, method: "POST", authorized: authorized)
        request.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
        var body = Data()
        body.append(Data("--\(boundary)\r\n".utf8))
        body.append(Data("Content-Disposition: form-data; name=\"\(fieldName)\"; filename=\"\(filename)\"\r\n".utf8))
        body.append(Data("Content-Type: \(mimeType)\r\n\r\n".utf8))
        body.append(fileData)
        body.append(Data("\r\n--\(boundary)--\r\n".utf8))
        request.httpBody = body
        return try await send(request)
    }

    private func makeRequest(path: String, method: String, authorized: Bool) throws -> URLRequest {
        guard let url = URL(string: path, relativeTo: baseURL)?.absoluteURL else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if authorized {
            guard let accessToken, !accessToken.isEmpty else { throw APIError.unauthorized }
            request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        }
        return request
    }

    private func send<T: Decodable>(_ request: URLRequest) async throws -> T {
        let (data, response) = try await session.data(for: request)
        try validate(response: response, data: data)
        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            throw APIError.decoding
        }
    }

    private func sendNoContent(_ request: URLRequest) async throws {
        let (data, response) = try await session.data(for: request)
        try validate(response: response, data: data)
    }

    private func validate(response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else { throw APIError.decoding }
        if http.statusCode == 401 { throw APIError.unauthorized }
        if http.statusCode == 501 {
            let hint = (try? JSONSerialization.jsonObject(with: data) as? [String: Any])?["hint"] as? String
            throw APIError.notImplemented(hint)
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8)
            throw APIError.http(http.statusCode, body)
        }
    }
}
