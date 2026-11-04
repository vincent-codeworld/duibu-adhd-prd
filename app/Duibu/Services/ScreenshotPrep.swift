import UIKit

enum ScreenshotPrepError: LocalizedError {
    case encodeFailed
    case tooLarge

    var errorDescription: String? {
        switch self {
        case .encodeFailed: return "Couldn’t prepare the photo."
        case .tooLarge: return "Photo is still over 4MB after compression — try another."
        }
    }
}

enum ScreenshotPrep {
    static let maxBytes = 4 * 1024 * 1024
    static let longEdge: CGFloat = 1600

    /// Resize (long edge ≤1600) and JPEG-encode. UIImage JPEG output drops EXIF.
    static func jpegData(from image: UIImage) throws -> Data {
        let resized = resize(image, longEdge: longEdge)
        var quality: CGFloat = 0.82
        var data = resized.jpegData(compressionQuality: quality)
        while let d = data, d.count > maxBytes, quality > 0.35 {
            quality -= 0.12
            data = resized.jpegData(compressionQuality: quality)
        }
        guard let data else { throw ScreenshotPrepError.encodeFailed }
        guard data.count <= maxBytes else { throw ScreenshotPrepError.tooLarge }
        return data
    }

    private static func resize(_ image: UIImage, longEdge: CGFloat) -> UIImage {
        let size = image.size
        let longest = max(size.width, size.height)
        guard longest > longEdge, longest > 0 else { return image }
        let scale = longEdge / longest
        let newSize = CGSize(width: size.width * scale, height: size.height * scale)
        let renderer = UIGraphicsImageRenderer(size: newSize)
        return renderer.image { _ in
            image.draw(in: CGRect(origin: .zero, size: newSize))
        }
    }
}
