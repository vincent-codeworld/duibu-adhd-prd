import XCTest
@testable import Duibu

final class GoogleSignInContractTests: XCTestCase {
    func testGoogleSignInContract() {
        #if DEBUG
        GoogleSignInContractChecks.runAll(requireSources: false)
        #else
        XCTFail("Google sign-in contract checks are part of the Debug app.")
        #endif
    }
}
