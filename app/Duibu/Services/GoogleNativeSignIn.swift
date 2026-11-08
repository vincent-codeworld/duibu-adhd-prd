import GoogleSignIn
import UIKit

/// Native Google Sign-In. The StepBuddy session is updated only after `POST /v1/auth/google` succeeds.
/// Do not read the Google profile email or send an auth code. Accounts are not linked by email.
enum GoogleNativeSignIn {
    @MainActor
    static func signIn(configuration: GoogleSignInConfiguration) async -> GoogleSignInAttempt {
        switch configuration.availability {
        case .unavailable(let message):
            return .unavailable(message)
        case .ready(let clientID, let serverClientID):
            let gidConfig: GIDConfiguration
            if let serverClientID {
                gidConfig = GIDConfiguration(clientID: clientID, serverClientID: serverClientID)
            } else {
                gidConfig = GIDConfiguration(clientID: clientID)
            }
            // Replace any Info.plist configuration, including an empty shipped default.
            GIDSignIn.sharedInstance.configuration = gidConfig
            guard let presenter = presenter() else {
                return .failed(GoogleSignInCopy.noPresenter)
            }
            do {
                let result = try await GIDSignIn.sharedInstance.signIn(withPresenting: presenter)
                let rawToken = result.user.idToken?.tokenString ?? ""
                let identityToken = rawToken.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !identityToken.isEmpty else {
                    return .failed(GoogleSignInCopy.noIdentityToken)
                }
                // Display name only. Email is intentionally not read.
                let suggested = result.user.profile?.name
                return .identity(token: identityToken, suggestedNickname: suggested)
            } catch {
                if isCancellation(error) {
                    return .cancelled
                }
                return .failed(error.localizedDescription)
            }
        }
    }

    /// Completes an in-flight OAuth redirect. This does not create a StepBuddy session.
    @MainActor
    static func handle(_ url: URL) -> Bool {
        GIDSignIn.sharedInstance.handle(url)
    }

    /// Drops the Google user from the keychain. The app session is cleared separately.
    @MainActor
    static func signOut() {
        GIDSignIn.sharedInstance.signOut()
    }

    @MainActor
    private static func presenter() -> UIViewController? {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        let scene = scenes.first { $0.activationState == .foregroundActive } ?? scenes.first
        let window = scene?.windows.first { $0.isKeyWindow } ?? scene?.windows.first
        return topViewController(from: window?.rootViewController)
    }

    @MainActor
    private static func topViewController(from base: UIViewController?) -> UIViewController? {
        if let navigation = base as? UINavigationController {
            return topViewController(from: navigation.visibleViewController)
        }
        if let tab = base as? UITabBarController {
            return topViewController(from: tab.selectedViewController)
        }
        if let presented = base?.presentedViewController {
            return topViewController(from: presented)
        }
        return base
    }

    private static func isCancellation(_ error: Error) -> Bool {
        let nsError = error as NSError
        return nsError.domain == kGIDSignInErrorDomain
            && nsError.code == GIDSignInError.canceled.rawValue
    }
}
