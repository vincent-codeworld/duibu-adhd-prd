import Foundation

/// JSON body shared by `POST /v1/auth/apple` and `POST /v1/auth/google`.
/// Nickname is omitted when nil. Email is never encoded.
struct IdentityTokenLoginBody: Encodable, Equatable {
    let identityToken: String
    let nickname: String?

    enum CodingKeys: String, CodingKey {
        case identityToken
        case nickname
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(identityToken, forKey: .identityToken)
        try container.encodeIfPresent(nickname, forKey: .nickname)
    }
}

enum AuthExchangePath {
    static let dev = "/v1/auth/dev"
    static let apple = "/v1/auth/apple"
    static let google = "/v1/auth/google"
}

enum LoginNickname {
    static let maxLength = 20

    /// Typed name wins. A provider display name is only a fallback. Email is not accepted here.
    static func resolved(typed: String?, suggested: String?) -> String? {
        let typedName = sanitize(typed)
        if !typedName.isEmpty {
            return String(typedName.prefix(maxLength))
        }
        let suggestedName = sanitize(suggested)
        guard !suggestedName.isEmpty else { return nil }
        return String(suggestedName.prefix(maxLength))
    }

    private static func sanitize(_ value: String?) -> String {
        value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }
}

enum GoogleSignInCopy {
    static let missingClientID = "Google sign-in is unavailable. This build has no iOS OAuth client ID. Set GOOGLE_IOS_CLIENT_ID and GOOGLE_IOS_URL_SCHEME in Config/GoogleSignIn.local.xcconfig."
    static let missingURLScheme = "Google sign-in is unavailable. Set GOOGLE_IOS_URL_SCHEME to the reversed iOS client ID."
    static let mismatchedURLScheme = "Google sign-in is unavailable. GOOGLE_IOS_URL_SCHEME does not match the reversed iOS client ID."
    static let invalidServerClientID = "Google sign-in is unavailable. GOOGLE_IOS_SERVER_CLIENT_ID is set but is not a Google client ID."
    static let cancelled = "Google sign-in cancelled."
    static let appleCancelled = "Sign in cancelled."
    static let noIdentityToken = "No Google identityToken received."
    static let noPresenter = "Google sign-in is unavailable. Could not find a window to present from."
    static let debugStubButton = "Debug stub POST /v1/auth/google"
    static let debugStubExplanation = "Simulator contract test only. Sends the dev subject as identityToken to POST /v1/auth/google. This does not contact Google and is not a Google sign-in."
    static let loginUploadExplanation = "Signing in tries to upload device-only tasks to your account. If an upload fails, the task stays on this phone."
    static let appleDeviceHint = "On device, Sign in with Apple uses AUTH_MODE=apple and APPLE_CLIENT_ID=com.stepbuddy.app."
    static let googleExchangeHint = "Google sign-in uses the iOS SDK, then sends only the identity token and an optional nickname to POST /v1/auth/google. This app does not link Apple and Google accounts by email."
}

/// Outcome of the native Google UI. Only `.identity` may be exchanged. Nothing here writes the app session.
enum GoogleSignInAttempt: Equatable {
    case cancelled
    case unavailable(String)
    case failed(String)
    case identity(token: String, suggestedNickname: String?)

    func exchangeRequest(typedNickname: String?) -> GoogleExchangeRequest? {
        guard case .identity(let token, let suggested) = self else { return nil }
        return GoogleExchangeRequest.real(
            token: token,
            typedNickname: typedNickname,
            suggestedNickname: suggested
        )
    }
}

struct GoogleExchangeRequest: Equatable {
    let identityToken: String
    let nickname: String?
    let path: String
    /// False for the debug stub. Callers must not describe that path as Google sign-in.
    let isRealGoogleSignIn: Bool

    var body: IdentityTokenLoginBody {
        IdentityTokenLoginBody(identityToken: identityToken, nickname: nickname)
    }

    static func real(token: String, typedNickname: String?, suggestedNickname: String?) -> GoogleExchangeRequest? {
        let trimmed = token.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        return GoogleExchangeRequest(
            identityToken: trimmed,
            nickname: LoginNickname.resolved(typed: typedNickname, suggested: suggestedNickname),
            path: AuthExchangePath.google,
            isRealGoogleSignIn: true
        )
    }

    static func debugStub(subject: String, typedNickname: String?) -> GoogleExchangeRequest? {
        let trimmed = subject.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        return GoogleExchangeRequest(
            identityToken: trimmed,
            nickname: LoginNickname.resolved(typed: typedNickname, suggested: nil),
            path: AuthExchangePath.google,
            isRealGoogleSignIn: false
        )
    }
}

struct GoogleSignInConfiguration: Equatable {
    var clientID: String
    var reversedClientID: String
    var serverClientID: String

    /// Placeholder URL scheme shipped when no reversed client ID has been set.
    /// It is not an OAuth client id and must not start the Google flow.
    static let unconfiguredURLScheme = "com.stepbuddy.app.google-signin-unconfigured"

    enum Availability: Equatable {
        case ready(clientID: String, serverClientID: String?)
        case unavailable(message: String)
    }

    var availability: Availability {
        Self.availability(clientID: clientID, reversedClientID: reversedClientID, serverClientID: serverClientID)
    }

    static func load(bundle: Bundle = .main) -> GoogleSignInConfiguration {
        load(infoDictionary: bundle.infoDictionary ?? [:])
    }

    static func load(infoDictionary: [String: Any]) -> GoogleSignInConfiguration {
        GoogleSignInConfiguration(
            clientID: string(infoDictionary["GIDClientID"]),
            reversedClientID: preferredURLScheme(from: infoDictionary),
            serverClientID: string(infoDictionary["GIDServerClientID"])
        )
    }

    static func reversedClientID(for clientID: String) -> String {
        clientID.split(separator: ".").reversed().joined(separator: ".")
    }

    static func availability(clientID: String, reversedClientID: String, serverClientID: String) -> Availability {
        let client = sanitize(clientID)
        let reversed = sanitize(reversedClientID)
        let server = sanitize(serverClientID)

        if client.isEmpty || isUnset(client) || !isGoogleClientID(client) {
            return .unavailable(message: GoogleSignInCopy.missingClientID)
        }
        if reversed.isEmpty || reversed == unconfiguredURLScheme || isUnset(reversed) {
            return .unavailable(message: GoogleSignInCopy.missingURLScheme)
        }
        if reversed != GoogleSignInConfiguration.reversedClientID(for: client) {
            return .unavailable(message: GoogleSignInCopy.mismatchedURLScheme)
        }
        if !server.isEmpty && !isUnset(server) {
            if !isGoogleClientID(server) {
                return .unavailable(message: GoogleSignInCopy.invalidServerClientID)
            }
            return .ready(clientID: client, serverClientID: server)
        }
        return .ready(clientID: client, serverClientID: nil)
    }

    static func isGoogleClientID(_ value: String) -> Bool {
        let suffix = ".apps.googleusercontent.com"
        guard value.hasSuffix(suffix), !isUnset(value) else { return false }
        let head = value.dropLast(suffix.count)
        guard let dash = head.firstIndex(of: "-"), dash != head.startIndex else { return false }
        let project = head[..<dash]
        let token = head[head.index(after: dash)...]
        guard !project.isEmpty, project.allSatisfy({ $0.isASCII && $0.isNumber }) else { return false }
        guard !token.isEmpty else { return false }
        let allowed = CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_")
        return token.unicodeScalars.allSatisfy { allowed.contains($0) }
    }

    private static func preferredURLScheme(from info: [String: Any]) -> String {
        let schemes = urlSchemes(from: info)
        if let configured = schemes.first(where: { sanitize($0) != unconfiguredURLScheme && !isUnset($0) }) {
            return sanitize(configured)
        }
        return schemes.first.map(sanitize) ?? ""
    }

    private static func urlSchemes(from info: [String: Any]) -> [String] {
        guard let types = info["CFBundleURLTypes"] as? [Any] else { return [] }
        var schemes: [String] = []
        for entry in types {
            guard let dict = entry as? [String: Any], let raw = dict["CFBundleURLSchemes"] as? [Any] else { continue }
            for item in raw {
                guard let scheme = item as? String else { continue }
                let trimmed = sanitize(scheme)
                if !trimmed.isEmpty { schemes.append(trimmed) }
            }
        }
        return schemes
    }

    private static func string(_ value: Any?) -> String {
        guard let value = value as? String else { return "" }
        return sanitize(value)
    }

    private static func sanitize(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func isUnset(_ value: String) -> Bool {
        if value.isEmpty || value.contains("$(") { return true }
        let lower = value.lowercased()
        let markers = ["your_", "your-", "placeholder", "replace_me", "replace-me", "changeme", "example"]
        return markers.contains { lower.contains($0) }
    }
}

#if DEBUG
enum GoogleSignInContractChecks {
    static func runAll(requireSources: Bool = false) {
        checkUnavailableDefaults()
        checkReadyConfiguration()
        checkExchangeBodies()
        checkSessionIsUntouchedUnlessIdentityExchanges()
        checkCopy()
        checkSources(require: requireSources)
    }

    private static func checkUnavailableDefaults() {
        let missing = GoogleSignInConfiguration.availability(clientID: "", reversedClientID: "", serverClientID: "")
        guard case .unavailable(let missingMessage) = missing else {
            fail("empty config should be unavailable")
        }
        check(missingMessage.contains("unavailable"), "missing client message")
        check(missingMessage.contains("GOOGLE_IOS_CLIENT_ID"), "missing client names the setting")

        let sentinel = GoogleSignInConfiguration.availability(
            clientID: "",
            reversedClientID: GoogleSignInConfiguration.unconfiguredURLScheme,
            serverClientID: ""
        )
        guard case .unavailable = sentinel else {
            fail("sentinel scheme is not a configured client")
        }

        let literal = GoogleSignInConfiguration.availability(
            clientID: "$(GOOGLE_IOS_CLIENT_ID)",
            reversedClientID: GoogleSignInConfiguration.unconfiguredURLScheme,
            serverClientID: "$(GOOGLE_IOS_SERVER_CLIENT_ID)"
        )
        guard case .unavailable = literal else {
            fail("unsubstituted build settings are not credentials")
        }

        let shapeOnly = "123456789012-abcDEF_123.apps.googleusercontent.com"
        let reversed = GoogleSignInConfiguration.reversedClientID(for: shapeOnly)
        check(reversed == "com.googleusercontent.apps.123456789012-abcDEF_123", "reversed client id")

        let mismatch = GoogleSignInConfiguration.availability(
            clientID: shapeOnly,
            reversedClientID: "com.googleusercontent.apps.other",
            serverClientID: ""
        )
        guard case .unavailable(let mismatchMessage) = mismatch else {
            fail("mismatched scheme should be unavailable")
        }
        check(mismatchMessage.contains("does not match"), "mismatch explains the scheme")

        let placeholder = GoogleSignInConfiguration.availability(
            clientID: "123456789012-exampletoken.apps.googleusercontent.com",
            reversedClientID: "com.googleusercontent.apps.123456789012-exampletoken",
            serverClientID: ""
        )
        guard case .unavailable = placeholder else {
            fail("placeholder-looking client ids stay unavailable")
        }
    }

    private static func checkReadyConfiguration() {
        let clientID = "123456789012-abcDEF_123.apps.googleusercontent.com"
        let reversed = GoogleSignInConfiguration.reversedClientID(for: clientID)
        let ready = GoogleSignInConfiguration.availability(
            clientID: clientID,
            reversedClientID: reversed,
            serverClientID: ""
        )
        guard case .ready(let readyClient, let readyServer) = ready else {
            fail("matching iOS client id and reversed scheme should be ready")
        }
        check(readyClient == clientID, "ready client id")
        check(readyServer == nil, "unset server client id stays nil so aud is the iOS client")

        let loaded = GoogleSignInConfiguration.load(infoDictionary: [
            "GIDClientID": clientID,
            "GIDServerClientID": "",
            "CFBundleURLTypes": [
                ["CFBundleURLSchemes": [GoogleSignInConfiguration.unconfiguredURLScheme, reversed]]
            ]
        ])
        guard case .ready(_, nil) = loaded.availability else {
            fail("info.plist load should ignore the unconfigured sentinel scheme")
        }

        let serverID = "999000111222-serverclient.apps.googleusercontent.com"
        let withServer = GoogleSignInConfiguration.availability(
            clientID: clientID,
            reversedClientID: reversed,
            serverClientID: serverID
        )
        guard case .ready(_, let server) = withServer, server == serverID else {
            fail("optional server client id should be passed through when it is a Google client id")
        }

        let badServer = GoogleSignInConfiguration.availability(
            clientID: clientID,
            reversedClientID: reversed,
            serverClientID: "not-a-client"
        )
        guard case .unavailable(let badServerMessage) = badServer else {
            fail("bad server client id should be unavailable")
        }
        check(badServerMessage.contains("GOOGLE_IOS_SERVER_CLIENT_ID"), "bad server client names the setting")
    }

    private static func checkExchangeBodies() {
        let tokenOnly = IdentityTokenLoginBody(identityToken: "token-value", nickname: nil)
        let withNick = IdentityTokenLoginBody(identityToken: "token-value", nickname: "Ada")
        check(keys(of: tokenOnly) == ["identityToken"], "nil nickname is omitted and email is absent")
        check(keys(of: withNick) == ["identityToken", "nickname"], "nickname is the only optional field")
        check(stringValue("identityToken", in: withNick) == "token-value", "identity token field")
        check(stringValue("nickname", in: withNick) == "Ada", "nickname field")

        let real = GoogleExchangeRequest.real(token: " google-jwt ", typedNickname: "  ", suggestedNickname: "Grace Hopper")
        check(real?.path == AuthExchangePath.google, "google path")
        check(real?.isRealGoogleSignIn == true, "sdk exchange is real Google sign-in")
        check(real?.nickname == "Grace Hopper", "display name fallback")
        check(real?.identityToken == "google-jwt", "trimmed token")

        let typed = GoogleExchangeRequest.real(
            token: "jwt",
            typedNickname: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
            suggestedNickname: "Other"
        )
        check(typed?.nickname == "ABCDEFGHIJKLMNOPQRST", "typed nickname wins and is capped at 20")

        let stub = GoogleExchangeRequest.debugStub(subject: " dev-jennie ", typedNickname: "Jennie")
        check(stub?.path == AuthExchangePath.google, "stub uses the google exchange path")
        check(stub?.isRealGoogleSignIn == false, "stub is not real Google sign-in")
        check(stub?.identityToken == "dev-jennie", "stub sends the dev subject")
        check(stub?.nickname == "Jennie", "stub nickname")
        check(keys(of: stub!.body) == ["identityToken", "nickname"], "stub body matches the apple/google shape")
        check(GoogleExchangeRequest.debugStub(subject: " ", typedNickname: nil) == nil, "blank stub subject")
        check(GoogleExchangeRequest.real(token: " ", typedNickname: nil, suggestedNickname: nil) == nil, "blank real token")
    }

    private static func checkSessionIsUntouchedUnlessIdentityExchanges() {
        let typed = "Ada"
        check(GoogleSignInAttempt.cancelled.exchangeRequest(typedNickname: typed) == nil, "cancel does not exchange")
        check(GoogleSignInAttempt.failed("nope").exchangeRequest(typedNickname: typed) == nil, "error does not exchange")
        check(
            GoogleSignInAttempt.unavailable(GoogleSignInCopy.missingClientID).exchangeRequest(typedNickname: typed) == nil,
            "unavailable does not exchange"
        )
        let request = GoogleSignInAttempt.identity(token: "jwt", suggestedNickname: "Ignored").exchangeRequest(typedNickname: typed)
        check(request?.isRealGoogleSignIn == true, "identity can exchange")
        check(request?.nickname == "Ada", "typed nickname still wins")
    }

    private static func checkCopy() {
        let upload = GoogleSignInCopy.loginUploadExplanation.lowercased()
        check(!upload.contains("never upload"), "login copy matches best-effort auto-upload")
        check(upload.contains("upload"), "login copy mentions upload")
        check(upload.contains("stays on this phone"), "failed upload stays on device")
        let stub = GoogleSignInCopy.debugStubExplanation
        check(stub.contains("not a Google sign-in"), "stub copy refuses a real Google success label")
        check(!GoogleSignInCopy.debugStubButton.contains("Sign in with Google"), "stub button is not the Google button")
        check(GoogleSignInCopy.googleExchangeHint.contains("does not link"), "no client-side email linking")
    }

    private static func checkSources(require: Bool) {
        let appRoot = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let native = appRoot.appendingPathComponent("Duibu/Services/GoogleNativeSignIn.swift")
        guard FileManager.default.fileExists(atPath: native.path) else {
            if require { fail("missing \(native.path)") }
            return
        }
        let nativeSource = read(native)
        check(nativeSource.contains("idToken"), "native flow reads the identity token")
        check(nativeSource.contains("signIn(withPresenting:"), "native flow uses the Google SDK")
        check(nativeSource.contains("handle("), "oauth callback is forwarded")
        check(!nativeSource.contains("profile?.email"), "client does not read Google email")
        check(!nativeSource.contains("serverAuthCode"), "client does not send the server auth code as a token")
        check(!nativeSource.contains("restorePreviousSignIn("), "google keychain restore does not replace the app session")

        let me = read(appRoot.appendingPathComponent("Duibu/Features/Me/MeView.swift"))
        check(!me.lowercased().contains("never upload"), "login screen no longer contradicts auto-upload")
        check(me.contains("login.apple"), "apple button remains")
        check(me.contains("login.stub"), "dev stub remains")
        check(me.contains("login.apple.stub"), "apple stub remains")
        check(me.contains("login.google"), "google entry remains visible when unconfigured")
        check(me.contains("login.google.stub"), "debug google stub identifier")
        check(me.contains("GoogleSignInCopy.debugStubButton"), "stub uses the explicit debug label")
        check(me.contains("loginWithGoogleDebugStub"), "stub calls the debug method")

        let app = read(appRoot.appendingPathComponent("Duibu/DuibuApp.swift"))
        check(app.contains("uploadDeviceOnlyTasksAfterLogin"), "successful login still auto-uploads")
        check(app.contains("loginWithGoogle"), "google exchange adopts the same session")
        check(app.contains("GoogleNativeSignIn.signOut()"), "logout clears the google user")
        check(app.contains("GoogleNativeSignIn.handle"), "open url restores the in-flight oauth callback")

        let api = read(appRoot.appendingPathComponent("Duibu/Networking/APIClient.swift"))
        check(api.contains("AuthExchangePath.google"), "client posts the google exchange")
        check(api.contains("IdentityTokenLoginBody"), "google body matches apple")

        let config = read(appRoot.appendingPathComponent("Config/GoogleSignIn.xcconfig"))
        check(config.contains("GOOGLE_IOS_CLIENT_ID="), "client id setting exists")
        check(!config.contains(".apps.googleusercontent.com"), "xcconfig does not embed a client id")
        check(config.contains(GoogleSignInConfiguration.unconfiguredURLScheme), "default scheme is the unconfigured sentinel")
    }

    private static func keys(of body: IdentityTokenLoginBody) -> Set<String> {
        Set(json(body).keys)
    }

    private static func stringValue(_ key: String, in body: IdentityTokenLoginBody) -> String? {
        json(body)[key] as? String
    }

    private static func json(_ body: IdentityTokenLoginBody) -> [String: Any] {
        do {
            let data = try JSONEncoder().encode(body)
            return (try JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
        } catch {
            fail("json encode failed: \(error)")
        }
    }

    private static func read(_ url: URL) -> String {
        (try? String(contentsOf: url, encoding: .utf8)) ?? ""
    }

    private static func check(_ condition: @autoclosure () -> Bool, _ message: String) {
        if !condition() { fail(message) }
    }

    private static func fail(_ message: String) -> Never {
        preconditionFailure("Google sign-in contract: \(message)")
    }
}
#endif
