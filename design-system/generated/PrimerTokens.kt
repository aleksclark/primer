package com.aleksclark.primer.designsystem

import androidx.compose.ui.graphics.Color

object PrimerTokens {
    object Dark {
        val surface = Color(0xFF0E1013)
        val surfaceRaised = Color(0xFF15181C)
        val rule = Color(0xFF262A2F)
        val ruleStrong = Color(0xFF3A3F45)
        val textMuted = Color(0xFF7C838C)
        val text = Color(0xFFFFFFFF)
        val accent = Color(0xFF5B8CFF)
        val accentHover = Color(0xFF86A9FF)
        val onAccent = Color(0xFF0E1013)
        val attention = Color(0xFFFF8A6B)
    }

    object Light {
        val surface = Color(0xFFFFFFFF)
        val surfaceRaised = Color(0xFFF4F5F6)
        val rule = Color(0xFFDDE0E3)
        val ruleStrong = Color(0xFFB8BDC2)
        val textMuted = Color(0xFF6A7178)
        val text = Color(0xFF0E1013)
        val accent = Color(0xFF2E62D8)
        val accentHover = Color(0xFF174AAE)
        val onAccent = Color(0xFFFFFFFF)
        val attention = Color(0xFFB8431F)
    }

    object Space {
        const val Value1 = 4
        const val Value2 = 8
        const val Value3 = 12
        const val Value4 = 16
        const val Value5 = 20
        const val Value6 = 24
        const val Value8 = 32
        const val Value12 = 48
        const val Value16 = 64
    }

    object Radius {
        const val Default = 0
    }

    object Rule {
        const val Default = 1
        const val Active = 2
        const val Progress = 3
    }

    object Focus {
        const val Width = 1
        const val Offset = 2
    }

    object Motion {
        const val Fast = 120
        const val Standard = 200
        const val Slow = 320
    }
}
