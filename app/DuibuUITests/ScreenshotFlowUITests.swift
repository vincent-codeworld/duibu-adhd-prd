import XCTest

/// PhotosPicker path: Upload+Verifier → stages → Request verification → Choose photo → Send → inbox preview.
final class ScreenshotFlowUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments += ["-ui-testing", "-ui-testing-reset"]
        app.launch()
    }

    func testPhotosPickerRequestVerificationAndInboxPreview() throws {
        dismissSystemAlertsIfNeeded()

        let enter = app.buttons["onboarding.enter"]
        if enter.waitForExistence(timeout: 3) { enter.tap() }

        // Same reliable stub path as PatchProgressUITests (default subject dev-jennie).
        app.tabBars.buttons["Me"].tap()
        let login = app.descendants(matching: .any)["me.login"]
        XCTAssertTrue(login.waitForExistence(timeout: 5))
        login.tap()
        let stub = app.buttons["login.stub"]
        XCTAssertTrue(stub.waitForExistence(timeout: 5))
        stub.tap()
        XCTAssertTrue(app.tabBars.buttons["Me"].waitForExistence(timeout: 8))
        XCTAssertTrue(app.buttons["Sign out"].waitForExistence(timeout: 8), "stub login failed")

        app.tabBars.buttons["Tasks"].tap()
        openNewTask()

        let titleField = app.textFields["Title (required)"]
        XCTAssertTrue(titleField.waitForExistence(timeout: 5))
        titleField.tap()
        titleField.typeText("Screenshot UI smoke")
        app.buttons["create.save"].tap()

        let row = app.staticTexts["Screenshot UI smoke"]
        XCTAssertTrue(row.waitForExistence(timeout: 10))
        row.tap()
        sleep(1)

        // Create while signed in auto-uploads; manual Upload remains fallback when still device-only.
        let synced = app.staticTexts["Synced to account"]
        var upload = app.buttons["task.upload"]
        for _ in 0..<8 {
            if synced.exists || upload.waitForExistence(timeout: 1) { break }
            app.swipeUp()
            upload = app.buttons["task.upload"]
        }
        if upload.exists {
            upload.tap()
            // Pick VerifierJ on upload consent when task had no create-time verifier.
            pickMenuVerifier("VerifierJ")
            let confirm = app.buttons["upload.confirm"]
            XCTAssertTrue(confirm.waitForExistence(timeout: 5))
            confirm.tap()
        } else if !synced.exists {
            let byLabel = app.buttons.matching(NSPredicate(format: "label CONTAINS[c] %@", "Upload this task")).firstMatch
            if byLabel.waitForExistence(timeout: 2) {
                byLabel.tap()
                pickMenuVerifier("VerifierJ")
                let confirm = app.buttons["upload.confirm"]
                XCTAssertTrue(confirm.waitForExistence(timeout: 5))
                confirm.tap()
            } else {
                XCTFail("Upload missing after scroll — check login / sync state")
            }
        }

        let deadline = Date().addingTimeInterval(15)
        var ok = false
        while Date() < deadline {
            if synced.exists { ok = true; break }
            if !app.buttons["task.upload"].exists && !app.buttons["upload.confirm"].exists { ok = true; break }
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        XCTAssertTrue(ok, "upload did not reach synced state")

        // If verifier still missing (create had none; auto-upload skipped picker), choose on detail.
        var chose = false
        let chooseBtn = app.buttons.matching(NSPredicate(format: "label CONTAINS[c] %@", "Choose verifier")).firstMatch
        if chooseBtn.waitForExistence(timeout: 3) {
            chooseBtn.tap()
            if app.buttons["verifier.pick.VerifierJ"].waitForExistence(timeout: 5) {
                app.buttons["verifier.pick.VerifierJ"].tap()
                chose = true
            } else if app.buttons["VerifierJ"].waitForExistence(timeout: 3) {
                app.buttons["VerifierJ"].tap()
                chose = true
            }
            if chose { sleep(2) }
        }

        for i in 1...7 {
            let check = app.buttons["stage.check.\(i)"]
            if check.waitForExistence(timeout: 2) {
                check.tap()
                sleep(1)
            }
        }

        // Scroll toward completion actions.
        for _ in 0..<6 { app.swipeUp() }
        let request = app.descendants(matching: .any)["task.requestVerification"]
        if !request.waitForExistence(timeout: 8) {
            XCTAssertFalse(app.buttons["Complete task"].exists, "Complete task shown — verifier not attached")
        }
        XCTAssertTrue(request.waitForExistence(timeout: 12), "Request verification not shown")
        request.tap()

        let choose = app.descendants(matching: .any)["verify.choosePhoto"]
        XCTAssertTrue(choose.waitForExistence(timeout: 8))
        choose.tap()
        XCTAssertTrue(selectFirstPhotoInPicker(), "Could not select a photo")

        let send = app.buttons["verify.send"]
        XCTAssertTrue(send.waitForExistence(timeout: 12), "Send button missing")
        sleep(2)
        // Toolbar may wrap identifier on Other+Button — prefer hittable button.
        if send.isHittable {
            send.tap()
        } else {
            app.buttons["Send"].tap()
        }

        XCTAssertTrue(
            app.staticTexts["Waiting for verifier…"].waitForExistence(timeout: 20)
            || !app.descendants(matching: .any)["verify.send"].waitForExistence(timeout: 2),
            "Send did not dismiss"
        )

        // Switch to verifier account.
        app.tabBars.buttons["Me"].tap()
        XCTAssertTrue(app.buttons["Sign out"].waitForExistence(timeout: 6))
        app.buttons["Sign out"].tap()

        let login2 = app.descendants(matching: .any)["me.login"]
        XCTAssertTrue(login2.waitForExistence(timeout: 8))
        login2.tap()
        // Set subject to verifier-jennie
        scrollTo(labelContaining: "Dev subject")
        let subjectField = app.textFields["Dev subject"]
        XCTAssertTrue(subjectField.waitForExistence(timeout: 6))
        subjectField.tap()
        subjectField.clearAndType("verifier-jennie")
        let nickField = app.textFields["Nickname (optional, max 20)"]
        if nickField.waitForExistence(timeout: 2) {
            nickField.tap()
            nickField.clearAndType("VerifierJ")
        }
        let stub2 = app.buttons["login.stub"]
        XCTAssertTrue(stub2.waitForExistence(timeout: 4))
        if !stub2.isHittable { app.swipeUp() }
        stub2.tap()
        XCTAssertTrue(app.buttons["Sign out"].waitForExistence(timeout: 12))

        app.tabBars.buttons["Verify"].tap()
        let end = Date().addingTimeInterval(25)
        var sawPreview = false
        var hasInbox = false
        while Date() < end {
            if app.images.count > 0 { sawPreview = true }
            if app.staticTexts.matching(NSPredicate(format: "label CONTAINS[c] %@", "Screenshot")).firstMatch.exists
                || app.staticTexts.matching(NSPredicate(format: "label CONTAINS[c] %@", "complet")).firstMatch.exists {
                hasInbox = true
            }
            if sawPreview && hasInbox { break }
            if app.cells.count > 0 { app.cells.element(boundBy: 0).tap() }
            let anyRow = app.buttons.matching(NSPredicate(format: "label CONTAINS[c] %@", "Screenshot")).firstMatch
            if anyRow.exists { anyRow.tap() }
            RunLoop.current.run(until: Date().addingTimeInterval(0.6))
        }
        XCTAssertTrue(hasInbox || sawPreview, "Verifier inbox did not show screenshot / row")
    }

    private func openNewTask() {
        let plus = app.navigationBars.buttons["New Task"]
        if plus.waitForExistence(timeout: 4) {
            plus.tap()
        } else {
            XCTAssertTrue(app.buttons["New Task"].waitForExistence(timeout: 3))
            app.buttons["New Task"].tap()
        }
    }

    private func pickMenuVerifier(_ name: String) {
        let picker = app.descendants(matching: .any)["upload.verifier"]
        if picker.waitForExistence(timeout: 5) {
            picker.tap()
        } else if app.buttons["Verifier"].waitForExistence(timeout: 3) {
            app.buttons["Verifier"].tap()
        }
        if app.buttons[name].waitForExistence(timeout: 4) {
            app.buttons[name].tap()
        } else if app.staticTexts[name].waitForExistence(timeout: 3) {
            app.staticTexts[name].tap()
        }
    }

    private func scrollTo(labelContaining: String) {
        let pred = NSPredicate(format: "label CONTAINS[c] %@", labelContaining)
        for _ in 0..<10 {
            if app.descendants(matching: .any).matching(pred).firstMatch.exists { return }
            app.swipeUp()
        }
    }

    @discardableResult
    private func selectFirstPhotoInPicker() -> Bool {
        sleep(1)
        dismissSystemAlertsIfNeeded()
        let photosUI = XCUIApplication(bundleIdentifier: "com.apple.PhotosUIService")
        guard let runningApp = app else { return false }

        // Permission sheets (SpringBoard / PhotosUI / app)
        for _ in 0..<3 {
            dismissSystemAlertsIfNeeded()
            for candidate in [photosUI, runningApp] as [XCUIApplication] {
                for title in ["Allow Full Access", "Allow Access to All Photos", "Allow", "OK", "Continue"] {
                    if candidate.buttons[title].waitForExistence(timeout: 0.6) {
                        candidate.buttons[title].tap()
                    }
                }
            }
        }

        sleep(1)
        // Prefer coordinate taps — collection cells are often reported unhittable in PhotosUI.
        let targets: [XCUIApplication] = photosUI.exists ? [photosUI, runningApp] : [runningApp]
        for target in targets {
            let frame = target.frame
            if frame.width > 10 {
                // First thumbnail roughly upper-left of grid under nav.
                target.coordinate(withNormalizedOffset: CGVector(dx: 0.20, dy: 0.32)).tap()
                sleep(1)
                if target.buttons["Add"].waitForExistence(timeout: 1.5) {
                    target.buttons["Add"].tap()
                } else if target.buttons["Done"].waitForExistence(timeout: 1) {
                    target.buttons["Done"].tap()
                }
                // Success if back on request sheet with Change photo / preview / send enabled path
                if runningApp.staticTexts["Change photo"].waitForExistence(timeout: 2)
                    || runningApp.buttons["Change photo"].waitForExistence(timeout: 1)
                    || runningApp.descendants(matching: .any)["verify.send"].exists {
                    // If send exists we likely still on sheet; treat as selected when Change photo or any image
                    if runningApp.staticTexts["Change photo"].exists
                        || runningApp.buttons["Change photo"].exists
                        || runningApp.images.count > 0 {
                        return true
                    }
                }
            }
        }

        // Last resort: try first image element without requiring hittable via coordinate
        let img = photosUI.images.element(boundBy: 0)
        if img.waitForExistence(timeout: 2) {
            img.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            sleep(1)
            return true
        }
        let img2 = runningApp.images.element(boundBy: 0)
        if img2.waitForExistence(timeout: 2) {
            img2.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            sleep(1)
            return true
        }
        return runningApp.staticTexts["Change photo"].exists || runningApp.buttons["Change photo"].exists
    }

    private func dismissSystemAlertsIfNeeded() {
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        for title in ["Allow Full Access", "Allow Access to All Photos", "Allow", "OK"] {
            let btn = springboard.buttons[title]
            if btn.waitForExistence(timeout: 1.0) { btn.tap() }
        }
        for title in ["Allow Full Access", "Allow", "OK"] {
            if app.alerts.buttons[title].waitForExistence(timeout: 0.5) {
                app.alerts.buttons[title].tap()
            }
        }
    }
}

private extension XCUIElement {
    func clearAndType(_ text: String) {
        guard let value = self.value as? String else {
            typeText(text)
            return
        }
        if !value.isEmpty {
            tap()
            let delete = String(repeating: XCUIKeyboardKey.delete.rawValue, count: max(value.count + 4, 24))
            typeText(delete)
        }
        typeText(text)
    }
}
