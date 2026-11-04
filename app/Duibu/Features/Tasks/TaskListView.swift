import SwiftUI

struct TaskListView: View {
    @EnvironmentObject private var store: LocalStore
    @State private var showingCreate = false

    private var active: [TaskItem] { store.tasks.filter { $0.status == .active } }
    private var completed: [TaskItem] { store.tasks.filter { $0.status == .completed } }

    var body: some View {
        NavigationStack {
            Group {
                if store.tasks.isEmpty {
                    ContentUnavailableView {
                        Label {
                            Text("Add something you want to move forward.")
                                .multilineTextAlignment(.center)
                        } icon: {
                            ZStack {
                                Circle()
                                    .fill(StepBuddyTheme.emptyStateWash)
                                    .frame(width: 72, height: 72)
                                Image(systemName: "checklist")
                                    .font(.title)
                                    .foregroundStyle(StepBuddyTheme.accent)
                            }
                        }
                    } description: {
                        Text("Break big goals into small stages — one step at a time.")
                            .foregroundStyle(.secondary)
                    } actions: {
                        Button {
                            showingCreate = true
                        } label: {
                            Label("New Task", systemImage: "plus")
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(StepBuddyTheme.accent)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(StepBuddyTheme.screenWash.ignoresSafeArea())
                } else {
                    List {
                        if !active.isEmpty {
                            Section {
                                ForEach(active) { task in
                                    NavigationLink(value: task.id) {
                                        TaskRowView(task: task)
                                    }
                                    .stepBuddyCardRow(tone: .accent)
                                }
                            } header: {
                                StepBuddySectionHeader(title: "In progress", systemImage: "flame.fill", tone: .accent)
                            }
                        }
                        if !completed.isEmpty {
                            Section {
                                ForEach(completed) { task in
                                    NavigationLink(value: task.id) {
                                        TaskRowView(task: task)
                                    }
                                    .stepBuddyCardRow(tone: .success)
                                }
                            } header: {
                                StepBuddySectionHeader(title: "Completed", systemImage: "checkmark.seal.fill", tone: .success)
                            }
                        }
                    }
                }
            }
            .navigationTitle("Tasks")
            .stepBuddyListChrome()
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button { showingCreate = true } label: {
                        Image(systemName: "plus.circle.fill")
                            .symbolRenderingMode(.palette)
                            .foregroundStyle(StepBuddyTheme.accent, StepBuddyTheme.accentSoft)
                    }
                    .accessibilityLabel("New Task")
                }
            }
            .navigationDestination(for: UUID.self) { id in
                if let task = store.task(id: id) {
                    TaskDetailView(taskId: task.id)
                } else {
                    Text("Task not found")
                }
            }
            .sheet(isPresented: $showingCreate) {
                CreateTaskView()
            }
        }
    }
}

struct TaskRowView: View {
    let task: TaskItem

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            ZStack {
                Circle()
                    .fill(task.status == .completed ? StepBuddyTheme.successSoft : StepBuddyTheme.accentSoft)
                    .frame(width: 40, height: 40)
                Circle()
                    .stroke(
                        task.status == .completed ? StepBuddyTheme.success.opacity(0.35) : StepBuddyTheme.accent.opacity(0.35),
                        lineWidth: 4
                    )
                    .frame(width: 40, height: 40)
                Circle()
                    .trim(from: 0, to: progressFraction)
                    .stroke(
                        task.status == .completed ? StepBuddyTheme.success : StepBuddyTheme.accent,
                        style: StrokeStyle(lineWidth: 4, lineCap: .round)
                    )
                    .rotationEffect(.degrees(-90))
                    .frame(width: 40, height: 40)
                Text(shortCount)
                    .font(.caption2.weight(.bold))
                    .foregroundStyle(task.status == .completed ? StepBuddyTheme.success : StepBuddyTheme.accent)
            }
            VStack(alignment: .leading, spacing: StepBuddyTheme.rowSpacing) {
                Text(task.title)
                    .font(.headline)
                Text(task.progressLabel)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                HStack(spacing: 8) {
                    Label(task.dueAt.formatted(date: .abbreviated, time: .shortened), systemImage: "calendar")
                        .font(.caption)
                        .foregroundStyle(StepBuddyTheme.secondaryAccent)
                    if task.syncState == .deviceOnly {
                        Text("On device")
                            .font(.caption2.weight(.semibold))
                            .padding(.horizontal, 7)
                            .padding(.vertical, 3)
                            .background(Capsule().fill(StepBuddyTheme.warningSoft))
                            .foregroundStyle(StepBuddyTheme.warning)
                    } else if task.hasVerifier {
                        Image(systemName: "person.badge.shield.checkmark.fill")
                            .font(.caption)
                            .foregroundStyle(StepBuddyTheme.secondaryAccent)
                            .accessibilityLabel("Has verifier")
                    }
                }
            }
        }
        .padding(.vertical, 6)
    }

    private var progressFraction: CGFloat {
        guard task.stageTotal > 0 else { return 0 }
        return CGFloat(task.checkedCount) / CGFloat(task.stageTotal)
    }

    private var shortCount: String {
        "\(task.checkedCount)/\(task.stageTotal)"
    }
}
