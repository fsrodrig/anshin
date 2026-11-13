plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.roborazzi)
}

android {
    namespace = "io.github.fsrodrig.anshin"
    compileSdk = 37

    defaultConfig {
        applicationId = "io.github.fsrodrig.anshin"
        minSdk = 26
        targetSdk = 37
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

    testOptions {
        unitTests {
            // Robolectric needs the merged resources and assets: the font, the brand drawables, the theme.
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

roborazzi {
    outputDir.set(layout.projectDirectory.dir("src/test/screenshots"))
}

// The theme tests read the palette sources and the brand resources outside this project. Declaring them as inputs
// reruns the tests when only those files change.
val branding = layout.projectDirectory.dir("../../branding")
tasks.withType<Test>().configureEach {
    // Robolectric sets up API 37 shared memory through jdk.internal.access, which JDK 21 does not export.
    jvmArgs("--add-exports=java.base/jdk.internal.access=ALL-UNNAMED")
    systemProperty("anshin.branding", branding.asFile.absolutePath)
    systemProperty("anshin.main", layout.projectDirectory.dir("src/main").asFile.absolutePath)
    inputs.files(
        fileTree(branding) { include("palette.json", "themes/*.json", "android/res/**") },
    ).withPathSensitivity(PathSensitivity.RELATIVE).withPropertyName("branding")
    inputs.dir(layout.projectDirectory.dir("src/main")).withPathSensitivity(PathSensitivity.RELATIVE)
        .withPropertyName("mainSources")
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
