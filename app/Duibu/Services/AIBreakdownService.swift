import Foundation

/// MVP AI breakdown: only title + notes; client enforces 8s hard timeout + editable template fallback.
protocol AIBreakdownServing {
    func breakdown(title: String, notes: String) async -> (titles: [String], source: StageSource)
}

/// Calls `POST /v1/ai/decompose` (OpenAPI v0.4.0). On transport/decode error → local template.
struct APIAIBreakdownService: AIBreakdownServing {
    let api: APIClient

    func breakdown(title: String, notes: String) async -> (titles: [String], source: StageSource) {
        let trimmedNotes = notes.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            let res = try await api.decompose(
                title: title.trimmingCharacters(in: .whitespacesAndNewlines),
                notes: trimmedNotes.isEmpty ? nil : trimmedNotes
            )
            let titles = res.stages
                .map { $0.title.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
            let source: StageSource = (res.source == "template") ? .template : .ai
            return (titles, source)
        } catch {
            return (StageTemplate.titles, .template)
        }
    }
}

/// Offline / preview stub that always returns the PRD template.
struct TemplateAIBreakdownService: AIBreakdownServing {
    func breakdown(title: String, notes: String) async -> (titles: [String], source: StageSource) {
        _ = title
        _ = notes
        try? await Task.sleep(nanoseconds: 300_000_000)
        return (StageTemplate.titles, .template)
    }
}

enum AIBreakdownGate {
    static let timeoutSeconds: TimeInterval = 8

    static func resolve(
        title: String,
        notes: String,
        service: AIBreakdownServing
    ) async -> (titles: [String], source: StageSource) {
        let work = Task { await service.breakdown(title: title, notes: notes) }
        let timeout = Task {
            try? await Task.sleep(nanoseconds: UInt64(timeoutSeconds * 1_000_000_000))
            work.cancel()
        }
        let result = await work.value
        timeout.cancel()

        let cleaned = result.titles
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty && $0.count <= 40 }

        if (3...7).contains(cleaned.count) {
            return (cleaned, result.source)
        }
        // Client-side 8s cancel / invalid payload → PRD template.
        return (StageTemplate.titles, .template)
    }
}
