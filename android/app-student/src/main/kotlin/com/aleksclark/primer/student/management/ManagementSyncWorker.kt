package com.aleksclark.primer.student.management

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.aleksclark.primer.student.StudentRuntime
import java.util.concurrent.TimeUnit

class ManagementSyncWorker(
    context: Context,
    params: WorkerParameters,
) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        return runCatching {
            StudentRuntime(applicationContext).syncManagement()
            Result.success()
        }.getOrDefault(Result.retry())
    }

    companion object {
        const val UNIQUE = "primer-student-management-sync"

        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<ManagementSyncWorker>(15, TimeUnit.MINUTES).build()
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                UNIQUE,
                ExistingPeriodicWorkPolicy.KEEP,
                request,
            )
        }
    }
}
