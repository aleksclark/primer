// Resolve shared Android/Kotlin plugins once, including modules outside android/.
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.android.library) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.compose.compiler) apply false
}

subprojects {
    configurations.configureEach {
        resolutionStrategy {
            force("org.jetbrains.kotlin:kotlin-stdlib:2.0.21")
            force("org.jetbrains.kotlin:kotlin-stdlib-jdk7:2.0.21")
            force("org.jetbrains.kotlin:kotlin-stdlib-jdk8:2.0.21")
            force("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.9.0")
            force("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
            force("org.jetbrains.kotlinx:kotlinx-serialization-core:1.7.3")
            force("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
            force("androidx.browser:browser:1.8.0")
        }
    }
}
