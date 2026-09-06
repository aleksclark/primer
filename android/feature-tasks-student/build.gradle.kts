plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.compose.compiler)
    alias(libs.plugins.kotlin.serialization)
}

val configuredApiOrigin = providers.gradleProperty("primerApiOrigin")
    .orElse(providers.environmentVariable("PRIMER_API_ORIGIN"))
    .getOrElse("")

val tasksClientRoot = rootProject.layout.projectDirectory.dir("../primer-tasks/clients/kotlin")
val generatedClient = tasksClientRoot.file(
    "src/main/kotlin/com/aleksclark/primertasks/generated/GeneratedTasksApi.kt",
)

abstract class RequireGeneratedTasksClientTask : DefaultTask() {
    @get:InputFile
    abstract val generatedFile: RegularFileProperty

    @TaskAction
    fun verify() {
        check(generatedFile.get().asFile.exists()) {
            "Missing ${generatedFile.get().asFile.absolutePath}. Run `make tasks-clients` from the repo root."
        }
    }
}

val requireGeneratedTasksClient by tasks.registering(RequireGeneratedTasksClientTask::class) {
    generatedFile.set(generatedClient)
}

android {
    namespace = "com.aleksclark.primer.student.tasks"
    compileSdk = 35

    defaultConfig {
        minSdk = 26
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        buildConfigField("String", "CONFIGURED_API_ORIGIN", configuredApiOrigin.quoteForBuildConfig())
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    sourceSets {
        getByName("main") {
            // Temporary bridge until the client worker lands :tasks-client.
            // Import the committed façade, never copy generated DTOs.
            kotlin.srcDir(tasksClientRoot.dir("src/main/kotlin"))
        }
    }
}

tasks.named("preBuild").configure { dependsOn(requireGeneratedTasksClient) }
tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    dependsOn(requireGeneratedTasksClient)
}

kotlin { jvmToolchain(17) }

dependencies {
    implementation(project(":core-ui"))
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.datastore.preferences)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.okhttp)
    implementation(libs.androidx.camera.camera2)
    implementation(libs.androidx.camera.lifecycle)
    implementation(libs.androidx.camera.view)
    implementation(libs.zxing.core)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.material3)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)

    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.androidx.test.runner)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}

private fun String.quoteForBuildConfig(): String = "\"${replace("\"", "\\\\\"")}\""
