plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.roborazzi)
}

// ADR 0007: the Release workflow sets the version from its tag, so source never holds one, and a build without the
// property is a development build. X·1,000,000 + Y·1,000 + Z keeps versionCode in SemVer order, so Android's
// downgrade refusal and Obtainium follow the release order; parts above 999 are refused rather than wrapped.
val anshinVersion = providers.gradleProperty("anshin.version").orNull?.let { version ->
    val parts = Regex("""(0|[1-9]\d{0,2})\.(0|[1-9]\d{0,2})\.(0|[1-9]\d{0,2})""").matchEntire(version)
        ?.groupValues?.drop(1)?.map(String::toInt)
    if (parts == null || parts.all { it == 0 }) {
        throw GradleException("anshin.version must be X.Y.Z with each part 0 to 999 and not 0.0.0, got '$version'")
    }
    version to parts[0] * 1_000_000 + parts[1] * 1_000 + parts[2]
}

android {
    namespace = "io.github.fsrodrig.anshin"
    compileSdk = 37

    defaultConfig {
        applicationId = "io.github.fsrodrig.anshin"
        minSdk = 26
        targetSdk = 37
        anshinVersion?.let { (name, code) ->
            versionName = name
            versionCode = code
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
        }
    }

    buildFeatures {
        compose = true
    }

    sourceSets {
        getByName("main") {
            // The launcher icon and the in-app mark come straight from the brand pack that branding/build.py
            // generates, so the app can never drift from it. BrandResourcesTest fails on any copy under src/*/res.
            res.directories += "../../branding/android/res"
        }
    }

    testOptions {
        unitTests {
            // Robolectric needs the merged resources and assets: the font, the brand pack, the theme.
            isIncludeAndroidResources = true
        }
    }

    lint {
        warningsAsErrors = true
        abortOnError = true
        // Dependabot owns version freshness. These checks would also make a passing build start failing on the
        // day a newer version is published.
        disable += setOf("GradleDependency", "NewerVersionAvailable", "AndroidGradlePluginVersion", "OldTargetApi")
    }
}

kotlin {
    jvmToolchain(21)
    compilerOptions {
        allWarningsAsErrors = true
    }
}

// The contract lives outside the Gradle project (ADR 0005). As a declared input, a fixture change reruns the tests.
val contractFixtures = layout.projectDirectory.dir("../../contract/fixtures")
tasks.withType<Test>().configureEach {
    inputs.dir(contractFixtures).withPathSensitivity(PathSensitivity.RELATIVE).withPropertyName("contractFixtures")
    systemProperty("anshin.contractFixtures", contractFixtures.asFile.absolutePath)
}

roborazzi {
    outputDir.set(layout.projectDirectory.dir("src/test/screenshots"))
}

// The theme tests read the palette sources and the brand resources outside this project. Declaring them as inputs
// reruns the tests when only those files change.
val branding = layout.projectDirectory.dir("../../branding")
tasks.withType<Test>().configureEach {
    // Robolectric sets up API 37 shared memory through jdk.internal.access, which JDK 21 does not export.
    jvmArgs("--add-exports=java.base/jdk.internal.access=ALL-UNNAMED")
    // On API 37 a NATIVE-graphics capture copies a frame HWUI already queued, not a fresh draw. Rendering state left
    // by earlier test classes in the same JVM can make that frame the bare window background, so each class gets its
    // own JVM.
    forkEvery = 1
    systemProperty("anshin.branding", branding.asFile.absolutePath)
    systemProperty("anshin.main", layout.projectDirectory.dir("src/main").asFile.absolutePath)
    inputs.files(
        fileTree(branding) { include("palette.json", "themes/*.json", "android/res/**") },
    ).withPathSensitivity(PathSensitivity.RELATIVE).withPropertyName("branding")
    inputs.dir(layout.projectDirectory.dir("src/main")).withPathSensitivity(PathSensitivity.RELATIVE)
        .withPropertyName("mainSources")
    inputs.files(fileTree(layout.projectDirectory.dir("src")) { include("*/res/**") })
        .withPathSensitivity(PathSensitivity.RELATIVE).withPropertyName("sourceSetResources")
}

// scripts/qa passes a run directory here so QaThemeSeedTest exports the theme seeds it round-trips. Only the unit
// test task sees it, and only when given; no app build reads it.
providers.gradleProperty("anshin.qaSeedDir").orNull?.let { seedDir ->
    tasks.withType<Test>().configureEach { systemProperty("anshin.qaSeedDir", seedDir) }
}

dependencies {
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.activity.compose)
    implementation(libs.lifecycle.runtime.compose)
    implementation(libs.datastore.preferences)
    implementation(libs.okhttp)
    implementation(libs.okhttp.coroutines)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)
    debugImplementation(libs.compose.ui.tooling)
    debugImplementation(libs.compose.ui.test.manifest)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(platform(libs.compose.bom))
    testImplementation(libs.compose.ui.test.junit4)
    testImplementation(libs.robolectric)
    // Roborazzi brings Espresso 3.5, which calls InputManager.getInstance(), gone from API 37.
    testImplementation(libs.espresso.core)
    testImplementation(libs.roborazzi)
    testImplementation(libs.roborazzi.compose)
    testImplementation(libs.roborazzi.junit.rule)
}
