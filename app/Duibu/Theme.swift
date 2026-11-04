import SwiftUI
import UIKit

enum StepBuddyTheme {
    /// Primary action / progress — warm coral (calm, not neon).
    static let accent = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 1.00, green: 0.55, blue: 0.42, alpha: 1)
            : UIColor(red: 0.93, green: 0.42, blue: 0.28, alpha: 1)
    })
    /// Soft blue for secondary hierarchy (verifier, sync, calendar).
    static let secondaryAccent = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.55, green: 0.72, blue: 0.95, alpha: 1)
            : UIColor(red: 0.35, green: 0.55, blue: 0.82, alpha: 1)
    })
    /// Mint for success / completed.
    static let success = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.40, green: 0.78, blue: 0.62, alpha: 1)
            : UIColor(red: 0.22, green: 0.60, blue: 0.48, alpha: 1)
    })
    static let warning = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.98, green: 0.78, blue: 0.40, alpha: 1)
            : UIColor(red: 0.85, green: 0.55, blue: 0.18, alpha: 1)
    })

    /// Soft fills (chips, rings track, header pills).
    static let accentSoft = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.40, green: 0.22, blue: 0.18, alpha: 1)
            : UIColor(red: 0.99, green: 0.90, blue: 0.86, alpha: 1)
    })
    static let secondarySoft = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.18, green: 0.26, blue: 0.38, alpha: 1)
            : UIColor(red: 0.88, green: 0.93, blue: 0.98, alpha: 1)
    })
    static let successSoft = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.14, green: 0.30, blue: 0.24, alpha: 1)
            : UIColor(red: 0.86, green: 0.95, blue: 0.90, alpha: 1)
    })
    static let warningSoft = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.35, green: 0.28, blue: 0.14, alpha: 1)
            : UIColor(red: 0.99, green: 0.94, blue: 0.84, alpha: 1)
    })

    /// Screen wash — peach → sky so white isn’t dominant.
    static let backgroundTop = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.12, green: 0.11, blue: 0.14, alpha: 1)
            : UIColor(red: 0.99, green: 0.94, blue: 0.90, alpha: 1)
    })
    static let backgroundBottom = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.10, green: 0.13, blue: 0.18, alpha: 1)
            : UIColor(red: 0.90, green: 0.94, blue: 0.98, alpha: 1)
    })
    /// Warm cream cards over the wash.
    static let card = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.18, green: 0.17, blue: 0.20, alpha: 0.92)
            : UIColor(red: 1.00, green: 0.98, blue: 0.96, alpha: 0.94)
    })
    static let cardElevated = Color(uiColor: UIColor { t in
        t.userInterfaceStyle == .dark
            ? UIColor(red: 0.22, green: 0.21, blue: 0.24, alpha: 1)
            : UIColor(red: 1.00, green: 0.99, blue: 0.97, alpha: 1)
    })

    /// Back-compat aliases.
    static let background = backgroundTop
    static let secondaryText = Color.secondary

    static let sectionSpacing: CGFloat = 12
    static let rowSpacing: CGFloat = 6
    static let cardCorner: CGFloat = 14

    static var screenWash: LinearGradient {
        LinearGradient(
            colors: [backgroundTop, backgroundBottom],
            startPoint: .topLeading,
            endPoint: .bottomTrailing
        )
    }

    static var emptyStateWash: LinearGradient {
        LinearGradient(
            colors: [accentSoft.opacity(0.85), secondarySoft.opacity(0.9), successSoft.opacity(0.7)],
            startPoint: .topLeading,
            endPoint: .bottomTrailing
        )
    }
}

enum StepBuddySectionTone {
    case accent, secondary, success, warning, neutral

    var color: Color {
        switch self {
        case .accent: return StepBuddyTheme.accent
        case .secondary: return StepBuddyTheme.secondaryAccent
        case .success: return StepBuddyTheme.success
        case .warning: return StepBuddyTheme.warning
        case .neutral: return .secondary
        }
    }

    var soft: Color {
        switch self {
        case .accent: return StepBuddyTheme.accentSoft
        case .secondary: return StepBuddyTheme.secondarySoft
        case .success: return StepBuddyTheme.successSoft
        case .warning: return StepBuddyTheme.warningSoft
        case .neutral: return StepBuddyTheme.card
        }
    }
}

struct SoftCardModifier: ViewModifier {
    func body(content: Content) -> some View {
        content
            .listStyle(.insetGrouped)
            .scrollContentBackground(.hidden)
            .background(StepBuddyTheme.screenWash.ignoresSafeArea())
    }
}

struct StepBuddyCardBackground: View {
    var tone: StepBuddySectionTone = .neutral
    var body: some View {
        RoundedRectangle(cornerRadius: StepBuddyTheme.cardCorner, style: .continuous)
            .fill(StepBuddyTheme.card)
            .overlay(
                RoundedRectangle(cornerRadius: StepBuddyTheme.cardCorner, style: .continuous)
                    .strokeBorder(tone.soft.opacity(0.85), lineWidth: 1)
            )
            .shadow(color: Color.black.opacity(0.04), radius: 4, y: 2)
    }
}

struct StepBuddySectionHeader: View {
    let title: String
    let systemImage: String
    var tone: StepBuddySectionTone = .accent

    var body: some View {
        Label {
            Text(title)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(tone.color)
        } icon: {
            Image(systemName: systemImage)
                .foregroundStyle(tone.color)
                .padding(5)
                .background(Circle().fill(tone.soft))
        }
        .textCase(nil)
    }
}

extension View {
    func stepBuddyListChrome() -> some View {
        modifier(SoftCardModifier())
    }

    func stepBuddyCardRow(tone: StepBuddySectionTone = .neutral) -> some View {
        listRowBackground(StepBuddyCardBackground(tone: tone))
            .listRowSeparatorTint(tone.soft.opacity(0.5))
    }

    func stepBuddyFormChrome() -> some View {
        stepBuddyListChrome()
    }
}
