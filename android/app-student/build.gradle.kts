plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.compose.compiler)
}

// Student custody is deliberately independent from the existing TV signing key.
val storePath = providers.environmentVariable("PRIMER_STUDENT_KEYSTORE")
val signingStorePassword = providers.environmentVariable("PRIMER_STUDENT_STORE_PASSWORD")
val signingKeyAlias = providers.environmentVariable("PRIMER_STUDENT_KEY_ALIAS")
val signingKeyPassword = providers.environmentVariable("PRIMER_STUDENT_KEY_PASSWORD")
val signingValues = listOf(storePath.orNull, signingStorePassword.orNull, signingKeyAlias.orNull, signingKeyPassword.orNull)
check(signingValues.all { it != null } || signingValues.all { it == null }) {
    "Set all four PRIMER_STUDENT signing variables, or none."
}

val configuredApiOrigin = providers.gradleProperty("primerApiOrigin")
    .orElse(providers.environmentVariable("PRIMER_API_ORIGIN"))
    .getOrElse("")

android {
    namespace = "com.aleksclark.primer.student"
    compileSdk = 35
    defaultConfig {
        applicationId = "com.aleksclark.primer.student"
        minSdk = 28
        targetSdk = 35
        versionCode = providers.gradleProperty("studentVersionCode").orElse("1").get().toInt().also {
            require(it > 0)
        }
        versionName = providers.gradleProperty("studentVersionName").orElse("0.1.0-qualification").get()
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        buildConfigField("String", "CONFIGURED_API_ORIGIN", "\"${configuredApiOrigin.replace("\"", "\\\"")}\"")
    }
    signingConfigs {
        if (signingValues.all { it != null }) {
            create("student") {
                storeFile = file(storePath.get())
                storePassword = signingStorePassword.get()
                keyAlias = signingKeyAlias.get()
                keyPassword = signingKeyPassword.get()
            }
        }
    }
    buildTypes {
        release {
            if (signingValues.all { it != null }) signingConfig = signingConfigs.getByName("student")
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
}
kotlin { jvmToolchain(17) }

abstract class RequireStudentSigningTask : DefaultTask() {
    @get:Input
    abstract val signingConfigured: Property<Boolean>

    @TaskAction
    fun verifySigning() {
        check(signingConfigured.get()) { "Student release APK requires explicit signing custody." }
    }
}
val requireStudentSigning by tasks.registering(RequireStudentSigningTask::class) {
    signingConfigured.set(signingValues.all { it != null })
}
tasks.matching { it.name == "packageRelease" || it.name == "bundleRelease" }.configureEach {
    dependsOn(requireStudentSigning)
}

dependencies {
    implementation(project(":core-ui"))
    implementation(project(":core-device-policy"))
    implementation(project(":core-updates"))
    implementation(project(":feature-tasks-student"))
    implementation(project(":feature-device-management"))
    implementation(project(":core-security"))
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.kotlinx.coroutines.android)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.material3)
    testImplementation(libs.junit)
}
