pluginManagement {
    repositories {
        mavenCentral()
        gradlePluginPortal()
    }
    plugins {
        id("org.jetbrains.kotlin.jvm") version "2.0.21"
        id("org.jetbrains.kotlin.plugin.serialization") version "2.0.21"
        id("org.gradle.toolchains.foojay-resolver-convention") version "0.9.0"
    }
}

plugins {
    id("org.gradle.toolchains.foojay-resolver-convention")
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories { mavenCentral() }
}

rootProject.name = "tasks-client"
