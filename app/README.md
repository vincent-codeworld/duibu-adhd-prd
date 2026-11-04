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

Minimum: iOS 17. Set your Development Team in Xcode before device runs.

## Next

1. Real Sign in with Apple on device (Team ID + AUTH_MODE=apple)
2. Optional: deeper GUI smoke of screenshot end-to-end against live API
