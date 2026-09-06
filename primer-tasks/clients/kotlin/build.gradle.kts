plugins {
    kotlin("jvm") version "2.0.21"
    kotlin("plugin.serialization") version "2.0.21"
}

group = "com.aleksclark.primertasks"
version = "0.1.0"

kotlin { jvmToolchain(17) }

abstract class RequireGeneratedClient : DefaultTask() {
    @get:InputFile
    abstract val generatedFile: RegularFileProperty

    @TaskAction
    fun verify() {
        check(generatedFile.get().asFile.exists()) {
            "Missing generated Kotlin client at ${generatedFile.get().asFile.path}. Generate from a producer OpenAPI first."
        }
    }
}

val requireGeneratedClient by tasks.registering(RequireGeneratedClient::class) {
    generatedFile.set(layout.projectDirectory.file("src/main/kotlin/com/aleksclark/primertasks/generated/GeneratedTasksApi.kt"))
}

tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    dependsOn(requireGeneratedClient)
}

sourceSets {
    named("main") {
        kotlin.srcDir("src/main/kotlin")
    }
    named("test") {
        kotlin.srcDir("src/test/kotlin")
    }
}

dependencies {
    api("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.9.0")
    api("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    api("com.squareup.okhttp3:okhttp:4.12.0")
    testImplementation("junit:junit:4.13.2")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
}

tasks.test {
    useJUnit()
}
