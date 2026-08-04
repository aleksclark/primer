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

val primerVersionCode = providers.gradleProperty("primerVersionCode")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_VERSION_CODE"))
    .orElse("1")
val primerVersionName = providers.gradleProperty("primerVersionName")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_VERSION_NAME"))
    .orElse("0.1.0")
val releaseStoreFile = providers.gradleProperty("primerSigningStoreFile")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_KEYSTORE"))
val releaseStorePassword = providers.gradleProperty("primerSigningStorePassword")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_STORE_PASSWORD"))
val releaseKeyAlias = providers.gradleProperty("primerSigningKeyAlias")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_KEY_ALIAS"))
val releaseKeyPassword = providers.gradleProperty("primerSigningKeyPassword")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_KEY_PASSWORD"))
val releaseSigningValues = listOf(
    releaseStoreFile.orNull,
    releaseStorePassword.orNull,
    releaseKeyAlias.orNull,
    releaseKeyPassword.orNull,
)
check(releaseSigningValues.all { it != null } || releaseSigningValues.all { it == null }) {
    "Release signing requires all PRIMER_ANDROID_KEYSTORE, PRIMER_ANDROID_STORE_PASSWORD, " +
        "PRIMER_ANDROID_KEY_ALIAS, and PRIMER_ANDROID_KEY_PASSWORD values."
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
        versionCode = primerVersionCode.get().toInt().also {
            require(it > 0) { "primerVersionCode must be a positive integer" }
        }
        versionName = primerVersionName.get().also {
            require(it.isNotBlank()) { "primerVersionName must not be blank" }
        }

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        ndk {
            abiFilters += listOf("arm64-v8a", "armeabi-v7a")
        }
    }

    signingConfigs {
        if (releaseSigningValues.all { it != null }) {
            create("production") {
                storeFile = file(releaseStoreFile.get())
                storePassword = releaseStorePassword.get()
                keyAlias = releaseKeyAlias.get()
                keyPassword = releaseKeyPassword.get()
            }
        }
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
        }
        release {
            isMinifyEnabled = false
            if (releaseSigningValues.all { it != null }) {
                signingConfig = signingConfigs.getByName("production")
            }
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

    implementation(libs.coil.compose)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}
