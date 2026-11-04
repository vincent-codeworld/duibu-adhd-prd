import SwiftUI
import PhotosUI

struct RequestVerificationView: View {
    @EnvironmentObject private var appModel: AppModel
    @EnvironmentObject private var store: LocalStore
    @Environment(\.dismiss) private var dismiss

    let taskId: UUID
    let rewardId: UUID
    let rewardTitle: String

    @State private var note = ""
    @State private var isBusy = false
    @State private var errorMessage: String?
    @State private var pickerItem: PhotosPickerItem?
    @State private var previewImage: UIImage?
    @State private var preparedJPEG: Data?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(rewardTitle)
                    Text("Verifiers can’t see task notes — put anything they need here.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    TextField("Note for verifier (optional)", text: $note, axis: .vertical)
                        .lineLimit(2...5)
                        .onChange(of: note) { _, value in
                            if value.count > 200 { note = String(value.prefix(200)) }
                        }
                }

                Section("Screenshot (optional)") {
                    PhotosPicker(selection: $pickerItem, matching: .images) {
                        Label(
                            previewImage == nil ? "Choose photo" : "Change photo",
                            systemImage: "photo"
                        )
                    }
                    .accessibilityIdentifier("verify.choosePhoto")
                    .onChange(of: pickerItem) { _, item in
                        Task { await loadPhoto(item) }
                    }
                    if let previewImage {
                        Image(uiImage: previewImage)
                            .resizable()
                            .scaledToFit()
                            .frame(maxHeight: 180)
                            .clipShape(RoundedRectangle(cornerRadius: 8))
                        Button("Remove photo", role: .destructive) {
                            pickerItem = nil
                            self.previewImage = nil
                            preparedJPEG = nil
                        }
                    }
                    Text("We’ll strip location EXIF and shrink long edge to 1600px (≤4MB).")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                if let errorMessage {
                    Section { Text(errorMessage).foregroundStyle(.red) }
                }
            }
            .navigationTitle("Request verification")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isBusy ? "Sending…" : "Send") {
                        Task { await send() }
                    }
                    .accessibilityIdentifier("verify.send")
                    .disabled(isBusy)
                }
            }
        }
    }

    private func loadPhoto(_ item: PhotosPickerItem?) async {
        guard let item else { return }
        errorMessage = nil
        do {
            guard let data = try await item.loadTransferable(type: Data.self),
                  let image = UIImage(data: data) else {
                errorMessage = "Couldn’t read that photo."
                return
            }
            let jpeg = try ScreenshotPrep.jpegData(from: image)
            previewImage = image
            preparedJPEG = jpeg
        } catch {
            preparedJPEG = nil
            previewImage = nil
            errorMessage = error.localizedDescription
        }
    }

    private func send() async {
        isBusy = true
        defer { isBusy = false }
        do {
            await appModel.api.setToken(store.accessToken)
            guard let task = store.task(id: taskId) else { return }
            let reward = task.stages.compactMap(\.reward).first { $0.id == rewardId }
                ?? task.completionReward.flatMap { $0.id == rewardId ? $0 : nil }
            let remoteRewardId = reward?.remoteId ?? rewardId.uuidString

            var screenshotKey: String?
            if let jpeg = preparedJPEG {
                let uploaded = try await appModel.api.uploadScreenshot(jpegData: jpeg)
                screenshotKey = uploaded.screenshotKey
            }

            _ = try await appModel.api.submitVerification(
                rewardId: remoteRewardId,
                note: note.isEmpty ? nil : note,
                screenshotKey: screenshotKey
            )
            let remote = try await appModel.api.getTask(id: task.apiId)
            appModel.taskService.applySyncedRemote(remote, onto: task)
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
