import SwiftUI

struct RootView: View {
    @EnvironmentObject private var store: LocalStore

    var body: some View {
        Group {
            if store.hasSeenMedicalDisclaimer {
                MainTabView()
            } else {
                MedicalDisclaimerView()
            }
        }
        .tint(StepBuddyTheme.accent)
    }
}

struct MainTabView: View {
    var body: some View {
        TabView {
            TaskListView()
                .tabItem { Label("Tasks", systemImage: "checklist") }
            VerificationHubView()
                .tabItem { Label("Verify", systemImage: "person.2.fill") }
            MeView()
                .tabItem { Label("Me", systemImage: "person.crop.circle.fill") }
        }
        .toolbarBackground(.ultraThinMaterial, for: .tabBar)
    }
}
