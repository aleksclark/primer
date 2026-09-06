plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.compose.compiler)
}

val primerVersionCode = providers.gradleProperty("primerVersionCode")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_VERSION_CODE"))
    .orElse("1")
val primerVersionName = providers.gradleProperty("primerVersionName")
    .orElse(providers.environmentVariable("PRIMER_ANDROID_VERSION_NAME"))
    .orElse("0.1.0")
val releaseTrustRoot = providers.gradleProperty("primerReleaseTrustRoot")
    .orElse(providers.environmentVariable("PRIMER_RELEASE_TRUST_ROOT"))
    .getOrElse("")
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
        buildConfigField("String", "RELEASE_TRUST_ROOT", "\"${releaseTrustRoot.replace("\"", "\\\"")}\"")

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
        buildConfig = true
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
    sourceSets.getByName("test").java.srcDir(project(":core-updates").file("src/testShared/kotlin"))
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation(project(":core"))
    implementation(project(":core-ui"))
    implementation(project(":core-updates"))

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
