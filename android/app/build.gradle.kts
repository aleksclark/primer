plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.compose.compiler)
}

// Bridge System C tokens into the app package. Source of truth is
// design-system/generated/PrimerTokens.kt — never edit the generated output.
abstract class GeneratePrimerTokensTask : DefaultTask() {
    @get:InputFile
    abstract val sourceFile: RegularFileProperty

    @get:OutputDirectory
    abstract val outputDir: DirectoryProperty

    @get:Input
    abstract val targetPackage: Property<String>

    @TaskAction
    fun generate() {
        val source = sourceFile.get().asFile
        check(source.exists()) {
            "Missing ${source.absolutePath}. Run `make design-system` from the repo root."
        }
        val pkg = targetPackage.get()
        val packagePath = pkg.replace('.', '/')
        val outFile = outputDir.get().asFile.resolve("$packagePath/PrimerTokens.kt")
        outFile.parentFile.mkdirs()
        val rewritten = source.readText()
            .replace(
                Regex("""^package\s+[\w.]+""", RegexOption.MULTILINE),
                "package $pkg",
            )
        outFile.writeText(
            buildString {
                appendLine("// GENERATED from design-system/generated/PrimerTokens.kt — do not edit.")
                appendLine("// Regenerate via the generatePrimerTokens Gradle task (runs on preBuild).")
                appendLine()
                append(rewritten.trimStart())
                if (!rewritten.endsWith("\n")) appendLine()
            },
        )
    }
}

val primerTokensOutDir = layout.buildDirectory.dir("generated/primerTokens")
val generatePrimerTokens by tasks.registering(GeneratePrimerTokensTask::class) {
    group = "design system"
    description = "Copy PrimerTokens.kt into the app package from design-system/generated"
    sourceFile.set(
        rootProject.layout.projectDirectory.file("../design-system/generated/PrimerTokens.kt"),
    )
    outputDir.set(primerTokensOutDir)
    targetPackage.set("com.aleksclark.primer.tv.app.ui.designsystem")
}

android {
    namespace = "com.aleksclark.primer.tv"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.aleksclark.primer.tv"
        // Android 9 is the floor: the T9 / RK3318 box ships Pie and will never
        // be updated.
        minSdk = 28
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        ndk {
            abiFilters += listOf("arm64-v8a", "armeabi-v7a")
        }
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
        }
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    buildFeatures {
        compose = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }

    sourceSets {
        getByName("main") {
            kotlin.srcDir(primerTokensOutDir)
        }
    }
}

tasks.named("preBuild").configure {
    dependsOn(generatePrimerTokens)
}
// Unit tests / IDE sync can compile without a full preBuild.
tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    dependsOn(generatePrimerTokens)
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation(project(":core"))

    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.datastore.preferences)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.extended)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    implementation(libs.androidx.tv.material)

    implementation(libs.media3.exoplayer)
    implementation(libs.media3.ui)
    implementation(libs.media3.common)
    implementation(libs.media3.datasource.okhttp)
    // Official Media3 FFmpeg extension (LGPL audio: ac3/eac3/dca), vendored for CI.
    // Rebuild: android/third_party/media3-ffmpeg/build-media3-ffmpeg.sh
    implementation(
        files("libs/media3-ffmpeg-decoder-1.4.1.aar"),
    )
    // decoder_ffmpeg AAR depends on media3-decoder (and already pulls exoplayer via our deps).
    implementation(libs.media3.decoder)

    implementation(libs.coil.compose)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}
