plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.compose.compiler)
}

// Bridge System C tokens once for the shared UI library. Source of truth is
// design-system/generated/PrimerTokens.kt — never edit generated output.
abstract class GeneratePrimerUiTokensTask : DefaultTask() {
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
                appendLine("// Regenerate via :core-ui:generatePrimerUiTokens (runs on preBuild/Kotlin compile).")
                appendLine()
                append(rewritten.trimStart())
                if (!rewritten.endsWith("\n")) appendLine()
            },
        )
    }
}

val primerTokensOutDir = layout.buildDirectory.dir("generated/primerUiTokens")
val generatePrimerUiTokens by tasks.registering(GeneratePrimerUiTokensTask::class) {
    group = "design system"
    description = "Copy PrimerTokens.kt into the shared UI package from design-system/generated"
    sourceFile.set(rootProject.layout.projectDirectory.file("../design-system/generated/PrimerTokens.kt"))
    outputDir.set(primerTokensOutDir)
    targetPackage.set("com.aleksclark.primer.ui.tokens")
}

android {
    namespace = "com.aleksclark.primer.ui"
    compileSdk = 35

    defaultConfig {
        minSdk = 26
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildFeatures {
        compose = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    sourceSets {
        getByName("main") {
            kotlin.srcDir(primerTokensOutDir)
        }
    }
}

tasks.named("preBuild").configure {
    dependsOn(generatePrimerUiTokens)
}
tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    dependsOn(generatePrimerUiTokens)
}

kotlin { jvmToolchain(17) }

dependencies {
    implementation(libs.androidx.core.ktx)
    api(platform(libs.compose.bom))
    api(libs.compose.ui)
    api(libs.compose.material3)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)
    implementation(libs.zxing.core)

    testImplementation(libs.junit)
    testImplementation(libs.zxing.core)
}
