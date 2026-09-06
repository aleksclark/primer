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
val releaseTrustRoot = providers.gradleProperty("primerReleaseTrustRoot")
    .orElse(providers.environmentVariable("PRIMER_RELEASE_TRUST_ROOT"))
    .getOrElse("")

// Control custody is independent from TV and Student signing identities.
val storePath = providers.environmentVariable("PRIMER_CONTROL_KEYSTORE")
val signingStorePassword = providers.environmentVariable("PRIMER_CONTROL_STORE_PASSWORD")
val signingKeyAlias = providers.environmentVariable("PRIMER_CONTROL_KEY_ALIAS")
val signingKeyPassword = providers.environmentVariable("PRIMER_CONTROL_KEY_PASSWORD")
val signingValues = listOf(
    storePath.orNull,
    signingStorePassword.orNull,
    signingKeyAlias.orNull,
    signingKeyPassword.orNull,
)
check(signingValues.all { it != null } || signingValues.all { it == null }) {
    "Set all four PRIMER_CONTROL signing variables, or none."
}

val controlVersionCode = providers.gradleProperty("controlVersionCode")
    .orElse(providers.environmentVariable("PRIMER_CONTROL_VERSION_CODE"))
    .orElse("1")
val controlVersionName = providers.gradleProperty("controlVersionName")
    .orElse(providers.environmentVariable("PRIMER_CONTROL_VERSION_NAME"))
    .orElse("0.1.0")

android {
    namespace = "com.aleksclark.primer.control"
    compileSdk = 36
    defaultConfig {
        applicationId = "com.aleksclark.primer.control"
        minSdk = 28
        targetSdk = 35
        versionCode = controlVersionCode.get().toInt().also {
            require(it > 0) { "controlVersionCode must be a positive integer" }
        }
        versionName = controlVersionName.get().also {
            require(it.isNotBlank()) { "controlVersionName must not be blank" }
        }
        buildConfigField("String", "CONFIGURED_API_ORIGIN", configuredApiOrigin.quoteForBuildConfig())
        buildConfigField("String", "CLERK_PUBLISHABLE_KEY", clerkPublishableKey.quoteForBuildConfig())
        buildConfigField("String", "RELEASE_TRUST_ROOT", releaseTrustRoot.quoteForBuildConfig())
    }
    signingConfigs {
        if (signingValues.all { it != null }) {
            create("control") {
                storeFile = file(storePath.get())
                storePassword = signingStorePassword.get()
                keyAlias = signingKeyAlias.get()
                keyPassword = signingKeyPassword.get()
            }
        }
    }
    buildTypes {
        release {
            if (signingValues.all { it != null }) {
                signingConfig = signingConfigs.getByName("control")
            }
            isMinifyEnabled = false
        }
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    testOptions { unitTests.isReturnDefaultValues = true }
}

kotlin { jvmToolchain(17) }

abstract class RequireControlSigningTask : DefaultTask() {
    @get:Input
    abstract val signingConfigured: Property<Boolean>

    @TaskAction
    fun verifySigning() {
        check(signingConfigured.get()) { "Control release APK requires explicit signing custody." }
    }
}

val requireControlSigning by tasks.registering(RequireControlSigningTask::class) {
    signingConfigured.set(signingValues.all { it != null })
}
tasks.matching { it.name == "packageRelease" || it.name == "bundleRelease" }.configureEach {
    dependsOn(requireControlSigning)
}

dependencies {
    implementation(project(":core-ui"))
    implementation(project(":core-parent-identity"))
    implementation(project(":feature-tasks-control"))
    implementation(project(":feature-device-control"))
    implementation(project(":core-updates"))
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
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}

private fun String.quoteForBuildConfig(): String = "\"${replace("\"", "\\\\\"")}\""
