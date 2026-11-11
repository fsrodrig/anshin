# Native Kotlin + Jetpack Compose for the phone app

The phone app is native Kotlin with Jetpack Compose, not Expo React Native + Effect, even though the owner's daily language is TypeScript/Effect. On Expo the security-critical network code (cleartext policy, the tailnet-only destination check, later Keystore signing) would still be Kotlin, just hidden in native glue and config plugins, which is the hardest code to review; native also gives a ~2 MB APK and ~160 ms first frame against ~16 MB and ~410 ms in a published same-app benchmark, which matters for an app whose promise is fast and lightweight.

## Considered Options

- **Expo React Native + Effect v4**: best reviewability for the owner and a free iOS path; rejected for footprint and because the parts that most need review would not be TypeScript anyway.
- **Flutter, Kotlin Multiplatform, PWA served by an agent**: rejected (new language with no leverage; sharing decisions before a second platform exists; no secure context over plain HTTP and no UI when the hosting machine is off).
