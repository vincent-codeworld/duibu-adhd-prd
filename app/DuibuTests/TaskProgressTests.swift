import XCTest
@testable import Duibu

@MainActor
final class TaskProgressTests: XCTestCase {
    func testCheckIncreasesProgressImmediately() throws {
        let store = LocalStore()
        // Isolate test data by wiping via creating fresh service on empty store is hard;
        // use in-memory by constructing stages and exercising pure helpers.
        var stages = StageTemplate.makeStages()
        var task = TaskItem(
            id: UUID(),
            ownerAccountId: nil,
            installId: store.install.installId,
            syncState: .deviceOnly,
            title: "整理书桌",
            notes: "",
            status: .active,
            stageSource: .manual,
            designatedVerifierAccountId: nil,
            stages: stages,
            completionReward: nil,
            dueAt: Date().addingTimeInterval(86400),
            reminderFireAt: nil,
            createdAt: Date(),
            updatedAt: Date(),
            completedAt: nil,
            uploadConsentAt: nil
        )
        XCTAssertEqual(task.checkedCount, 0)
        stages[0].progress = .done
        task.stages = stages
        XCTAssertEqual(task.checkedCount, 1)
        XCTAssertEqual(task.progressLabel, "1/3 done")
    }

    func testSkipDoesNotIncreaseNumerator() {
        var stages = StageTemplate.makeStages()
        stages[0].progress = .skipped
        let task = TaskItem(
            id: UUID(),
            ownerAccountId: nil,
            installId: UUID(),
            syncState: .deviceOnly,
            title: "示例",
            notes: "",
            status: .active,
            stageSource: .template,
            designatedVerifierAccountId: nil,
            stages: stages,
            completionReward: nil,
            dueAt: Date().addingTimeInterval(86400),
            reminderFireAt: nil,
            createdAt: Date(),
            updatedAt: Date(),
            completedAt: nil,
            uploadConsentAt: nil
        )
        XCTAssertEqual(task.checkedCount, 0)
        XCTAssertEqual(task.skippedCount, 1)
        XCTAssertTrue(task.progressLabel.contains("skipped 1"))
        XCTAssertFalse(task.canComplete)
    }

    func testAllSkippedCannotComplete() {
        let stages = StageTemplate.makeStages().map {
            StageItem(id: $0.id, position: $0.position, title: $0.title, progress: .skipped, reward: nil, doneAt: nil, skippedAt: Date())
        }
        let task = TaskItem(
            id: UUID(),
            ownerAccountId: nil,
            installId: UUID(),
            syncState: .deviceOnly,
            title: "示例",
            notes: "",
            status: .active,
            stageSource: .template,
            designatedVerifierAccountId: nil,
            stages: stages,
            completionReward: nil,
            dueAt: Date().addingTimeInterval(86400),
            reminderFireAt: nil,
            createdAt: Date(),
            updatedAt: Date(),
            completedAt: nil,
            uploadConsentAt: nil
        )
        XCTAssertFalse(task.canComplete)
    }

    func testCanCompleteWithOneCheckedAndRestSkipped() {
        var stages = StageTemplate.makeStages()
        stages[0].progress = .done
        stages[1].progress = .skipped
        stages[2].progress = .skipped
        let task = TaskItem(
            id: UUID(),
            ownerAccountId: nil,
            installId: UUID(),
            syncState: .deviceOnly,
            title: "示例",
            notes: "",
            status: .active,
            stageSource: .manual,
            designatedVerifierAccountId: nil,
            stages: stages,
            completionReward: nil,
            dueAt: Date().addingTimeInterval(86400),
            reminderFireAt: nil,
            createdAt: Date(),
            updatedAt: Date(),
            completedAt: nil,
            uploadConsentAt: nil
        )
        XCTAssertTrue(task.canComplete)
    }
}
