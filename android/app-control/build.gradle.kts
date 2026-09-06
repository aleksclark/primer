plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.compose.compiler)
}

val configuredApiOrigin = providers.gradleProperty("primerApiOrigin")
    .orElse(providers.environmentVariable("PRIMER_API_ORIGIN"))
    .getOrElse("")
val clerkPublishableKey = providers.gradleProperty("primerClerkPublishableKey")
    .orElse(providers.environmentVariable("PRIMER_CLERK_PUBLISHABLE_KEY"))
    .getOrElse("")

android {
    namespace = "com.aleksclark.primer.control"
    compileSdk = 35
    defaultConfig {
        applicationId = "com.aleksclark.primer.control"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
        buildConfigField("String", "CONFIGURED_API_ORIGIN", configuredApiOrigin.quoteForBuildConfig())
        buildConfigField("String", "CLERK_PUBLISHABLE_KEY", clerkPublishableKey.quoteForBuildConfig())
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

kotlin { jvmToolchain(17) }

// Clerk 0.1.31 POM: Kotlin 2.1.20 / serialization 1.9.0 / browser 1.9.0. Those
// artifacts ship Kotlin 2.2 metadata and browser 1.9 needs AGP 8.9. Pin only
// this Clerk consumer (with :core-parent-identity), not the rest of the tree.
configurations.configureEach {
    resolutionStrategy {
        force("org.jetbrains.kotlin:kotlin-stdlib:2.0.21")
        force("org.jetbrains.kotlin:kotlin-stdlib-jdk7:2.0.21")
        force("org.jetbrains.kotlin:kotlin-stdlib-jdk8:2.0.21")
        force("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.9.0")
        force("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
        force("org.jetbrains.kotlinx:kotlinx-serialization-core:1.7.3")
        force("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
        force("androidx.browser:browser:1.8.0")
    }
}

dependencies {
    implementation(project(":core-ui"))
    implementation(project(":core-parent-identity"))
    implementation(project(":feature-tasks-control"))
    implementation(project(":feature-device-control"))
    implementation(project(":tasks-client"))
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.kotlinx.coroutines.android)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.material3)
    testImplementation(libs.junit)
}

private fun String.quoteForBuildConfig(): String = "\"${replace("\"", "\\\\\"")}\""
