pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

// Allow jvmToolchain(17) to fetch a JDK when only a newer runtime is installed.
plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "0.9.0"
}

dependencyResolutionManagement {
    repositoriesMode = RepositoriesMode.FAIL_ON_PROJECT_REPOS
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "primer-tv"

include(":core")
include(":core-ui")
include(":app")
include(":app-student", ":core-device-policy", ":core-updates", ":feature-tasks-student")
