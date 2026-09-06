plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.serialization)
}
android {
    namespace = "com.aleksclark.primer.updates"
    compileSdk = 35
    defaultConfig { minSdk = 28 }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
kotlin { jvmToolchain(17) }

val generateReleaseManifest by tasks.registering(Exec::class) {
    workingDir = rootProject.projectDir.parentFile
    commandLine("node", "android/core-updates/generate-release-manifest.mjs")
    inputs.file(rootProject.projectDir.parentFile.resolve("primer-tasks/build/openapi.json"))
    outputs.dir(layout.buildDirectory.dir("generated/releaseManifest"))
}
android {
    sourceSets.getByName("main").java.srcDir(layout.buildDirectory.dir("generated/releaseManifest"))
}
tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    dependsOn(generateReleaseManifest)
}

dependencies {
    implementation(libs.kotlinx.serialization.json)
    testImplementation(libs.junit)
}
