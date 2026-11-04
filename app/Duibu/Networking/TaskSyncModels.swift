import Foundation

struct RemoteRewardInput: Encodable {
    let text: String
    let link: String?
}

struct RemoteStageInput: Encodable {
    let title: String
    let progress: String?
    let remindAt: Date?

    init(title: String, progress: String? = nil, remindAt: Date? = nil) {
        self.title = title
        self.progress = progress
        self.remindAt = remindAt
    }

    enum CodingKeys: String, CodingKey { case title, progress, remindAt }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(title, forKey: .title)
        try c.encodeIfPresent(progress, forKey: .progress)
        try c.encodeIfPresent(remindAt, forKey: .remindAt)
    }
}

struct CreateSyncedTaskRequest: Encodable {
    let title: String
    let notes: String
    let uploadConsent: Bool
    let dueAt: Date
    let stages: [RemoteStageInput]
    let stageRewards: [String: RemoteRewardInput]?
    let completionReward: RemoteRewardInput?
    let designatedVerifierAccountId: String?
}

struct RemoteRewardView: Decodable, Equatable {
    let id: String
    let text: String
    let link: String?
    let redeemState: String
}

struct RemoteStageView: Decodable, Equatable {
    let id: String
    let position: Int
    let title: String
    let progress: String
    let remindAt: Date?
    let reward: RemoteRewardView?
}

struct RemoteTaskView: Decodable, Equatable, Identifiable {
    let id: String
    let title: String
    let notes: String
    let status: String
    let designatedVerifierAccountId: String?
    let dueAt: Date
    let uploadConsentAt: Date
    let createdAt: Date
    let updatedAt: Date
    let completedAt: Date?
    let stages: [RemoteStageView]
    let completionReward: RemoteRewardView?
}

struct RemoteTasksList: Decodable {
    let tasks: [RemoteTaskView]
}


struct PatchTaskRequest: Encodable {
    var title: String? = nil
    var notes: String? = nil
    var dueAt: Date? = nil
    var stages: [RemoteStageInput]? = nil
    var stageRewards: [String: RemoteRewardInput]? = nil
    var completionReward: RemoteRewardInput? = nil
    var clearCompletionReward: Bool? = nil

    enum CodingKeys: String, CodingKey {
        case title, notes, dueAt, stages, stageRewards, completionReward, clearCompletionReward
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encodeIfPresent(title, forKey: .title)
        try c.encodeIfPresent(notes, forKey: .notes)
        try c.encodeIfPresent(dueAt, forKey: .dueAt)
        try c.encodeIfPresent(stages, forKey: .stages)
        try c.encodeIfPresent(stageRewards, forKey: .stageRewards)
        try c.encodeIfPresent(completionReward, forKey: .completionReward)
        try c.encodeIfPresent(clearCompletionReward, forKey: .clearCompletionReward)
    }
}

struct SetVerifierRequest: Encodable {
    let accountId: String
}

struct SubmitVerificationRequest: Encodable {
    let note: String?
    let screenshotKey: String?
}

struct VerificationInboxItem: Decodable, Identifiable, Equatable {
    var id: String { requestId }
    let requestId: String
    let rewardId: String
    let attempt: Int
    let status: String
    let note: String
    let screenshotKey: String?
    let screenshotUrl: String?
    let applicantNickname: String
    let taskTitle: String
    let stageTitle: String?
    let rewardText: String
    let rewardLink: String?
    let createdAt: Date
}

struct VerificationOutboxItem: Decodable, Identifiable, Equatable {
    var id: String { requestId }
    let requestId: String
    let rewardId: String
    let attempt: Int
    let status: String
    let note: String
    let resolutionNote: String?
    let createdAt: Date
    let resolvedAt: Date?
    let taskTitle: String
    let stageTitle: String?
    let rewardText: String
}

struct VerificationListResponse<T: Decodable>: Decodable {
    let items: [T]
}


// MARK: - Notices (OpenAPI v0.3.0)

struct RemoteNotice: Decodable, Identifiable, Equatable {
    let id: String
    let kind: String
    let requestId: String
    let createdAt: Date
    let readAt: Date?

    var isUnread: Bool { readAt == nil }

    var kindLabel: String {
        switch kind {
        case "verification_pending": return "Someone asked you to verify"
        case "verification_approved": return "Verification approved"
        case "verification_rejected": return "Verification rejected"
        case "verification_withdrawn": return "They withdrew the request"
        default: return kind
        }
    }
}

struct NoticesListResponse: Decodable {
    let items: [RemoteNotice]
    let unreadCount: Int
}


// MARK: - AI decompose (OpenAPI v0.4.0)

struct DecomposeRequest: Encodable {
    let title: String
    let notes: String?
}

struct DecomposeStageDTO: Decodable, Equatable {
    let title: String
}

struct DecomposeResponse: Decodable, Equatable {
    let stages: [DecomposeStageDTO]
    let source: String
}


// MARK: - Reports (OpenAPI v0.5.0)

struct CreateReportRequest: Encodable {
    let requestId: String
    let reason: String
    let detail: String?
}

struct ReportReceipt: Decodable, Equatable {
    let id: String
    let requestId: String
    let reason: String
    let detail: String?
    let createdAt: Date
    let message: String
}

// MARK: - Screenshots (OpenAPI v0.7.0)

struct ScreenshotUploadResponse: Decodable, Equatable {
    let screenshotKey: String
    let contentType: String
    let byteSize: Int
}
