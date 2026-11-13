plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
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

dependencies {
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.activity.compose)
    implementation(libs.lifecycle.runtime.compose)
    implementation(libs.okhttp)
    implementation(libs.okhttp.coroutines)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)
}
