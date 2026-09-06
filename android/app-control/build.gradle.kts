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
