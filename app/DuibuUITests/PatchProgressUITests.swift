import XCTest

final class PatchProgressUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments += ["-ui-testing", "-ui-testing-reset"]
        app.launch()
    }

    func testStubLoginUploadCheckSkipMatchesGET() throws {
        let enter = app.buttons["onboarding.enter"]
        if enter.waitForExistence(timeout: 3) { enter.tap() }

        app.tabBars.buttons["Me"].tap()
        let login = app.descendants(matching: .any)["me.login"]
        XCTAssertTrue(login.waitForExistence(timeout: 5))
        login.tap()
        let stub = app.buttons["login.stub"]
        XCTAssertTrue(stub.waitForExistence(timeout: 5))
        stub.tap()
        XCTAssertTrue(app.tabBars.buttons["Me"].waitForExistence(timeout: 8))

        app.tabBars.buttons["Tasks"].tap()
        let plus = app.navigationBars.buttons["New Task"]
        if plus.waitForExistence(timeout: 3) {
            plus.tap()
        } else if app.buttons["New Task"].waitForExistence(timeout: 2) {
            app.buttons["New Task"].tap()
        } else {
            XCTFail("no create button")
        }

        let titleField = app.textFields["Title (required)"]
        XCTAssertTrue(titleField.waitForExistence(timeout: 5))
        titleField.tap()
        titleField.typeText("GUI冒烟PATCH8")

        app.buttons["create.save"].tap()

        let row = app.staticTexts["GUI冒烟PATCH8"]
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        row.tap()
        sleep(1)

        // Create while signed in now auto-uploads; keep manual Upload path as fallback.
        let syncedLabel = app.staticTexts["Synced to account"]
        var upload = app.buttons["task.upload"]
        for _ in 0..<8 {
            if syncedLabel.exists || upload.waitForExistence(timeout: 1) { break }
            app.swipeUp()
            upload = app.buttons["task.upload"]
        }
        if upload.exists {
            upload.tap()
            let confirm = app.buttons["upload.confirm"]
            XCTAssertTrue(confirm.waitForExistence(timeout: 5))
            confirm.tap()
        }

        let stillUpload = app.buttons["task.upload"]
        let deadline = Date().addingTimeInterval(15)
        var ok = false
        while Date() < deadline {
            if syncedLabel.exists { ok = true; break }
            if !stillUpload.exists && !app.buttons["upload.confirm"].exists { ok = true; break }
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        XCTAssertTrue(ok, "upload did not reach synced state")

        let check1 = app.buttons["stage.check.1"]
        XCTAssertTrue(check1.waitForExistence(timeout: 5))
        check1.tap()
        sleep(1)

        let skip2 = app.buttons["stage.skip.2"]
        XCTAssertTrue(skip2.waitForExistence(timeout: 5))
        skip2.tap()
        // Under -ui-testing, skip confirms immediately (alert path still used in normal runs).
        // If alert appears anyway, confirm it.
        let confirmSkip = app.alerts.buttons["Skip for now"]
        if confirmSkip.waitForExistence(timeout: 2) {
            confirmSkip.tap()
        }
        sleep(2)

        // Verify via API on host (avoid nested URLSession+semaphore deadlock).
        let token = try Self.postJSON(
            url: "http://127.0.0.1:8080/v1/auth/dev",
            body: ["subject": "dev-jennie", "nickname": "Jennie"]
        )["accessToken"] as? String
        XCTAssertNotNil(token, "auth failed")
        let list = try Self.getJSON(
            url: "http://127.0.0.1:8080/v1/tasks",
            token: token!
        )
        let tasks = (list["tasks"] as? [[String: Any]]) ?? []
        guard let match = tasks.first(where: { ($0["title"] as? String) == "GUI冒烟PATCH8" }),
              let stages = match["stages"] as? [[String: Any]] else {
            XCTFail("task missing in GET: \(tasks.map { $0["title"] as? String })")
            return
        }
        let remoteProgress: [(String, String)] = stages.compactMap { s in
            guard let title = s["title"] as? String, let p = s["progress"] as? String else { return nil }
            return (title, p)
        }
        XCTAssertGreaterThanOrEqual(remoteProgress.count, 3, "\(remoteProgress)")
        XCTAssertEqual(remoteProgress[0].1, "done", "\(remoteProgress)")
        XCTAssertEqual(remoteProgress[1].1, "skipped", "\(remoteProgress)")
    }

    private static func postJSON(url: String, body: [String: String]) throws -> [String: Any] {
        var req = URLRequest(url: URL(string: url)!)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (data, _) = try syncData(for: req)
        return (try JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
    }

    private static func getJSON(url: String, token: String) throws -> [String: Any] {
        var req = URLRequest(url: URL(string: url)!)
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let (data, _) = try syncData(for: req)
        return (try JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
    }

    private static func syncData(for request: URLRequest) throws -> (Data, URLResponse) {
        let box = SyncBox()
        let sem = DispatchSemaphore(value: 0)
        URLSession.shared.dataTask(with: request) { data, resp, err in
            box.data = data
            box.resp = resp
            box.err = err
            sem.signal()
        }.resume()
        XCTAssertEqual(sem.wait(timeout: .now() + 10), .success, "network timeout \(request.url!)")
        if let err = box.err { throw err }
        return (box.data ?? Data(), box.resp!)
    }
}

private final class SyncBox: @unchecked Sendable {
    var data: Data?
    var resp: URLResponse?
    var err: Error?
}
