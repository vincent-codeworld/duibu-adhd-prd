#if DEBUG
@main
enum CheckGoogleSignInContract {
    static func main() {
        GoogleSignInContractChecks.runAll(requireSources: true)
        print("Google sign-in contract checks passed")
    }
}
#else
#error("Compile with -D DEBUG")
#endif
