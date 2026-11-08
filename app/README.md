# StepBuddy iOS (SwiftUI)

Local path: `/Users/vincent/app_projects/duibu-adhd/duibu-adhd-prd/app`  
Product requirements: same repo → `docs/PRD.md` ([duibu-adhd-prd](https://github.com/vincent-codeworld/duibu-adhd-prd))

Display name: **StepBuddy** · Bundle ID: `com.stepbuddy.app` · Default language: English (overseas).

## Generate

```bash
cd /Users/vincent/app_projects/duibu-adhd/duibu-adhd-prd/app
xcodegen generate
open Duibu.xcodeproj
```

Minimum: iOS 17. Set your Development Team in Xcode before device runs. The Apple Team ID is still unset and is not required to build this Google sign-in change.

`project.yml` is the XcodeGen spec. The checked-in `Duibu.xcodeproj` includes the same Google Sign-In package, config file, and Info.plist merge. After pulling, regenerate on a Mac with XcodeGen if you change the spec:

```bash
cd /Users/vincent/app_projects/duibu-adhd/duibu-adhd-prd/app
xcodegen generate
```

## Sign in

Apple and the dev stub are unchanged. A successful Apple, Google, or stub login still stores the app session and best-effort uploads device-only tasks. A failed upload leaves the task on the phone. Cancel, error, and a missing Google client ID do not clear a session that was already restored from local storage.

### Google (real)

The app uses Google Sign-In iOS SDK **10.1.0** (`GoogleSignIn` and `GoogleSignInSwift`) from `https://github.com/google/GoogleSignIn-iOS`. No OAuth client ID is in the repo. Until you add one, the login screen says Google sign-in is unavailable and does not start the SDK or call the API.

1. Google Cloud Console → APIs & Services → Credentials → OAuth client ID → **iOS**.
2. Bundle ID: `com.stepbuddy.app`.
3. Copy `Config/GoogleSignIn.local.xcconfig.example` to `Config/GoogleSignIn.local.xcconfig` (gitignored).
4. Set `GOOGLE_IOS_CLIENT_ID` to the iOS client ID (`….apps.googleusercontent.com`).
5. Set `GOOGLE_IOS_URL_SCHEME` to the reversed client ID (`com.googleusercontent.apps.<prefix>`). The app refuses to start Google sign-in when this does not match the client ID.
6. Leave `GOOGLE_IOS_SERVER_CLIENT_ID` empty unless the API expects the ID token `aud` to be a Web client ID. When it is empty, `aud` is the iOS client ID. When it is set, Google puts that server client ID in `aud`.

`Config/GoogleSignIn.xcconfig` is the checked-in default (`GOOGLE_IOS_CLIENT_ID` empty, URL scheme `com.stepbuddy.app.google-signin-unconfigured`). `Config/GoogleSignIn-Info.plist` is merged with the generated Info.plist and supplies `GIDClientID`, optional `GIDServerClientID`, and the callback URL scheme. Rebuild after changing the xcconfig.

The button is the official Google Sign-In button. It exchanges `GIDSignInResult.user.idToken.tokenString` only.

### Backend assumption

Mark’s `POST /v1/auth/google` was not merged when this client landed (cloud agent `bc-218c1ab2-b7ee-55bf-8cd2-4b84b5322b10` had no PR). The app assumes the same session payload as `POST /v1/auth/apple`:

```json
{ "identityToken": "<Google ID token>", "nickname": "optional, max 20" }
```

Response `AuthResponse`: `accessToken`, `expiresAt`, `created`, `account` (`id`, `nickname`, `inviteCode`, `createdAt`).

Assumed server behavior:

- Account identity is the Google `sub` inside the token, not the email.
- The client does not send email and does not link an Apple account to a Google account.
- `401` invalid token, `400` missing token, `404` if the route is not deployed yet, `503` if server-side Google auth is not configured. The app shows the API error and does not treat that as a successful Google sign-in.
- Debug stub below assumes `AUTH_MODE=stub` accepts the raw `identityToken` string as a stable subject, the same way stub Apple login does. If the merged route rejects non-JWT tokens, the stub will fail visibly.

App Check is not configured. The app does not call `GIDSignIn.configure` and does not ship an App Check debug key. Sign-in uses the SDK path that proceeds without an App Check token.

The app does not call `restorePreviousSignIn`. The StepBuddy session restored on launch is the token saved by `LocalStore`. `onOpenURL` only forwards the OAuth redirect back to the SDK so an in-flight Google sheet can finish. Logout also calls `GIDSignIn.signOut()` so a later Google sign-in does not reuse a keychain user.

### Debug stub (simulator contract test)

DEBUG builds show **Debug stub POST /v1/auth/google**. It sends the dev subject as `identityToken` to `POST /v1/auth/google`. It does not contact Google. A session from that button is the stub backend, not a successful Google sign-in. Release builds omit the button.

### Contract checks without Xcode

On a machine with Swift. Linux cannot run `xcodebuild` or the iOS simulator:

```bash
cd app
swiftc -D DEBUG \
  Duibu/Networking/IdentityLogin.swift \
  scripts/check_google_sign_in_contract.swift \
  -o /tmp/google-auth-check
/tmp/google-auth-check
```

Xcode unit test: `GoogleSignInContractTests` (Debug).

## Next

1. Real Sign in with Apple on device (Team ID + AUTH_MODE=apple)
2. Fill `GoogleSignIn.local.xcconfig` when an iOS OAuth client ID exists, then sign in against `POST /v1/auth/google`
3. Optional: deeper GUI smoke of screenshot end-to-end against live API
