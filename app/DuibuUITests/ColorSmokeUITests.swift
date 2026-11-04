import XCTest

/// Captures color-palette smoke shots for Jacky review (host-writable .smoke paths).
final class ColorSmokeUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments += ["-ui-testing", "-ui-testing-reset"]
        app.launch()
    }

    func testCaptureColorPaletteShots() throws {
        let smoke = URL(fileURLWithPath: "/Users/vincent/app_projects/duibu-adhd/duibu-adhd-prd/app/.smoke")
        try FileManager.default.createDirectory(at: smoke, withIntermediateDirectories: true)

        let enter = app.buttons["onboarding.enter"]
        if enter.waitForExistence(timeout: 3) { enter.tap() }

        // Seed one task so list + detail aren’t empty.
        openNewTask()
        let titleField = app.textFields["Title (required)"]
        XCTAssertTrue(titleField.waitForExistence(timeout: 5))
        // Capture New Task first (before save).
        sleep(1)
        try saveShot(named: "ui-color-create.png", to: smoke)

        titleField.tap()
        titleField.typeText("Color palette smoke")
        app.buttons["create.save"].tap()

        let row = app.staticTexts["Color palette smoke"]
        XCTAssertTrue(row.waitForExistence(timeout: 8))
        sleep(1)
        try saveShot(named: "ui-color-tasks.png", to: smoke)

        row.tap()
        sleep(1)
        // Scroll a bit so stages / sync chrome show.
        app.swipeUp()
        sleep(1)
        try saveShot(named: "ui-color-detail.png", to: smoke)
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

    private func saveShot(named: String, to dir: URL) throws {
        let shot = XCUIScreen.main.screenshot()
        let url = dir.appendingPathComponent(named)
        try shot.pngRepresentation.write(to: url)
        XCTAssertTrue(FileManager.default.fileExists(atPath: url.path), "missing \(named)")
    }
}
