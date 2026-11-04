import XCTest
@testable import Duibu

@MainActor
final class StageEditTests: XCTestCase {
    func testReplaceStagesRejectsFewerThanThree() async throws {
        let store = LocalStore()
        let reminder = ReminderService()
        let service = TaskService(store: store, reminder: reminder)
        let dueAt = Date().addingTimeInterval(86400)
        let task = try await service.createTask(
            title: "Mail the forms",
            notes: "",
            stages: StageTemplate.makeStages(),
            stageSource: .manual,
            dueAt: dueAt,
            verifierAccountId: nil
        )
        let tooFew = Array(task.stages.prefix(2))
        do {
            try await service.replaceStages(on: task, stages: tooFew, source: nil)
            XCTFail("expected invalidStageCount")
        } catch let error as TaskServiceError {
            XCTAssertEqual(error, .invalidStageCount)
        }
    }

    func testReplaceStagesKeepsOrderAfterMove() async throws {
        let store = LocalStore()
        let reminder = ReminderService()
        let service = TaskService(store: store, reminder: reminder)
        let dueAt = Date().addingTimeInterval(86400)
        let task = try await service.createTask(
            title: "Tidy the desk",
            notes: "",
            stages: StageTemplate.makeStages(),
            stageSource: .manual,
            dueAt: dueAt,
            verifierAccountId: nil
        )
        var stages = task.stages
        stages.swapAt(0, 1)
        try await service.replaceStages(on: task, stages: stages, source: nil)
        let updated = store.task(id: task.id)!
        XCTAssertEqual(updated.stages.map(\.title), ["Do a small chunk", "Prepare", "Wrap up"])
        XCTAssertEqual(updated.stages.map(\.position), [1, 2, 3])
    }
}
