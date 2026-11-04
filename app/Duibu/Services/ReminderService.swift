import Foundation
import UserNotifications

@MainActor
final class ReminderService {
    func requestAuthorizationIfNeeded() async -> Bool {
        let center = UNUserNotificationCenter.current()
        let settings = await center.notificationSettings()
        switch settings.authorizationStatus {
        case .authorized, .provisional, .ephemeral:
            return true
        case .denied:
            return false
        case .notDetermined:
            return (try? await center.requestAuthorization(options: [.alert, .sound])) ?? false
        @unknown default:
            return false
        }
    }

    func scheduleDue(taskId: UUID, title: String, dueAt: Date) async {
        await schedule(
            id: dueId(for: taskId),
            title: "Due: \(title)",
            body: "StepBuddy — this task is due.",
            fireAt: dueAt
        )
    }

    func scheduleStage(taskId: UUID, stageId: UUID, stageTitle: String, remindAt: Date) async {
        await schedule(
            id: stageNotifId(taskId: taskId, stageId: stageId),
            title: stageTitle,
            body: "StepBuddy — stage reminder.",
            fireAt: remindAt
        )
    }

    func cancelDue(for taskId: UUID) {
        UNUserNotificationCenter.current()
            .removePendingNotificationRequests(withIdentifiers: [dueId(for: taskId)])
    }

    func cancelStage(taskId: UUID, stageId: UUID) {
        UNUserNotificationCenter.current()
            .removePendingNotificationRequests(withIdentifiers: [stageNotifId(taskId: taskId, stageId: stageId)])
    }

    func cancelAll(for task: TaskItem) {
        var ids = [dueId(for: task.id)]
        ids.append(contentsOf: task.stages.map { stageNotifId(taskId: task.id, stageId: $0.id) })
        // Legacy single-reminder id
        ids.append("duibu.task.\(task.id.uuidString)")
        UNUserNotificationCenter.current().removePendingNotificationRequests(withIdentifiers: ids)
    }

    /// Reschedule due + all stage reminders for a task.
    func rescheduleAll(for task: TaskItem) async {
        cancelAll(for: task)
        guard task.status == .active else { return }
        await scheduleDue(taskId: task.id, title: task.title, dueAt: task.dueAt)
        for stage in task.stages {
            if let remindAt = stage.remindAt, stage.progress == .pending {
                await scheduleStage(
                    taskId: task.id,
                    stageId: stage.id,
                    stageTitle: stage.title,
                    remindAt: remindAt
                )
            }
        }
    }

    private func schedule(id: String, title: String, body: String, fireAt: Date) async {
        guard fireAt > Date() else { return }
        _ = await requestAuthorizationIfNeeded()
        UNUserNotificationCenter.current().removePendingNotificationRequests(withIdentifiers: [id])

        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default

        let comps = Calendar.current.dateComponents([.year, .month, .day, .hour, .minute], from: fireAt)
        let trigger = UNCalendarNotificationTrigger(dateMatching: comps, repeats: false)
        let request = UNNotificationRequest(identifier: id, content: content, trigger: trigger)
        try? await UNUserNotificationCenter.current().add(request)
    }

    private func dueId(for taskId: UUID) -> String {
        "stepbuddy.due.\(taskId.uuidString)"
    }

    private func stageNotifId(taskId: UUID, stageId: UUID) -> String {
        "stepbuddy.stage.\(taskId.uuidString).\(stageId.uuidString)"
    }
}
