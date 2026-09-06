plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "com.aleksclark.primertasks.client"
    compileSdk = 35
    defaultConfig { minSdk = 26 }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    sourceSets {
        getByName("main").java.srcDir("src/main/kotlin")
        getByName("test").java.srcDir("src/test/kotlin")
    }
}

kotlin { jvmToolchain(17) }

abstract class RequireGeneratedClient : DefaultTask() {
    @get:InputFile
    abstract val generatedFile: RegularFileProperty

    @TaskAction
    fun verify() {
        check(generatedFile.get().asFile.exists()) {
            "Missing generated Kotlin client at ${generatedFile.get().asFile.path}. Run `make tasks-clients` first."
        }
    }
}

val requireGeneratedClient by tasks.registering(RequireGeneratedClient::class) {
    generatedFile.set(layout.projectDirectory.file("src/main/kotlin/com/aleksclark/primertasks/generated/GeneratedTasksApi.kt"))
}

tasks.matching { it.name.startsWith("compile") && it.name.contains("Kotlin") }.configureEach {
    dependsOn(requireGeneratedClient)
}

dependencies {
    api(libs.kotlinx.coroutines.core)
    api(libs.kotlinx.serialization.json)
    api(libs.okhttp)
    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)
}
