import SwiftUI

struct MedicalDisclaimerView: View {
    @EnvironmentObject private var store: LocalStore

    var body: some View {
        ZStack {
            StepBuddyTheme.background.ignoresSafeArea()
            VStack(spacing: 28) {
                Spacer()
                Image(systemName: "checklist.checked")
                    .font(.system(size: 48, weight: .semibold))
                    .foregroundStyle(StepBuddyTheme.accent)
                Text("StepBuddy")
                    .font(.largeTitle.weight(.bold))
                Text("A calm notebook for small steps and agreements — not medical advice or treatment.")
                    .font(.body)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 36)
                Spacer()
                Button("Continue") {
                    store.acknowledgeMedicalDisclaimer()
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .tint(StepBuddyTheme.accent)
                .accessibilityIdentifier("onboarding.enter")
                .padding(.horizontal, 24)
                .padding(.bottom, 40)
            }
        }
    }
}
