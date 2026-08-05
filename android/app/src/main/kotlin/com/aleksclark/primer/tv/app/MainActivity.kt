package com.aleksclark.primer.tv.app

import android.app.UiModeManager
import android.content.Context
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.lifecycle.ViewModelProvider
import com.aleksclark.primer.tv.app.admin.KioskController
import com.aleksclark.primer.tv.app.ui.TvShell
import com.aleksclark.primer.tv.app.ui.TvViewModel
import com.aleksclark.primer.tv.core.domain.FormFactor

class MainActivity : ComponentActivity() {

    private lateinit var viewModel: TvViewModel
    private lateinit var kioskController: KioskController

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val application = application as TvApplication
        val container = application.container
        kioskController = application.kioskController
        viewModel = ViewModelProvider(this, TvViewModel.Factory(container))[TvViewModel::class.java]

        val formFactor = if (kioskController.isTelevision()) {
            FormFactor.TELEVISION
        } else {
            FormFactor.fromUiModeType(
                (getSystemService(Context.UI_MODE_SERVICE) as UiModeManager).currentModeType,
            )
        }

        setContent {
            TvShell(
                viewModel = viewModel,
                formFactor = formFactor,
                httpClient = container.httpClient,
                onExit = { if (!kioskController.isManagedTelevision()) finish() },
            )
        }
    }

    override fun onResume() {
        super.onResume()
        kioskController.enterLockTask(this)
    }

    /**
     * Backgrounding stops the heartbeat loop but keeps the session open, so a
     * film interrupted by the home button can be resumed on the same grant
     * rather than spending a second play.
     */
    override fun onStop() {
        super.onStop()
        viewModel.detachPlayer()
    }
}
