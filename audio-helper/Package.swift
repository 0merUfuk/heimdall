// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "heimdall-audio",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(
            name: "heimdall-audio",
            path: "Sources"
        )
    ]
)
