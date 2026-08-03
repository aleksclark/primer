package com.aleksclark.primer.tv.app.ui.scaffold

import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.background
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ViewList
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.LiveTv
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.NavigationRailItemDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface
import com.aleksclark.primer.tv.app.ui.navigation.TopLevelDestination
import com.aleksclark.primer.tv.core.domain.FormFactor

private data class NavItem(
    val destination: TopLevelDestination,
    val icon: ImageVector,
)

private val PrimaryNavItems = listOf(
    NavItem(TopLevelDestination.HOME, Icons.Outlined.Home),
    NavItem(TopLevelDestination.CHANNEL, Icons.Outlined.LiveTv),
    NavItem(TopLevelDestination.GUIDE, Icons.AutoMirrored.Outlined.ViewList),
)

private val AllNavItems = PrimaryNavItems + NavItem(TopLevelDestination.SETTINGS, Icons.Outlined.Settings)

/**
 * Top-level navigation chrome with form-factor-specific composition behind one
 * API. Content receives padding so screens can avoid the nav chrome.
 */
@Composable
fun StreamingScaffold(
    formFactor: FormFactor,
    selected: TopLevelDestination,
    onSelect: (TopLevelDestination) -> Unit,
    modifier: Modifier = Modifier,
    showSettingsInBottomBar: Boolean = false,
    content: @Composable (PaddingValues) -> Unit,
) {
    when (formFactor) {
        FormFactor.TELEVISION -> TelevisionScaffold(
            selected = selected,
            onSelect = onSelect,
            modifier = modifier,
            content = content,
        )

        FormFactor.TABLET -> PhoneTabletScaffold(
            selected = selected,
            onSelect = onSelect,
            modifier = modifier,
            showSettingsInBottomBar = showSettingsInBottomBar,
            content = content,
        )
    }
}

@Composable
private fun PhoneTabletScaffold(
    selected: TopLevelDestination,
    onSelect: (TopLevelDestination) -> Unit,
    modifier: Modifier,
    showSettingsInBottomBar: Boolean,
    content: @Composable (PaddingValues) -> Unit,
) {
    val colors = PrimerTheme.colors
    val useRail = LocalConfiguration.current.screenWidthDp >= 840
    val items = if (showSettingsInBottomBar || useRail) AllNavItems else PrimaryNavItems

    if (useRail) {
        Row(
            modifier = modifier
                .fillMaxSize()
                .background(colors.background),
        ) {
            NavigationRail(
                containerColor = colors.surface,
                contentColor = colors.onSurface,
                modifier = Modifier.fillMaxHeight(),
            ) {
                Spacer(Modifier.height(PrimerTheme.spacing.md))
                items.forEach { item ->
                    NavigationRailItem(
                        selected = item.destination == selected,
                        onClick = { onSelect(item.destination) },
                        icon = { Icon(item.icon, contentDescription = item.destination.label) },
                        label = { Text(item.destination.label) },
                        colors = NavigationRailItemDefaults.colors(
                            selectedIconColor = colors.onBrand,
                            selectedTextColor = colors.brand,
                            indicatorColor = colors.brand,
                            unselectedIconColor = colors.onSurfaceMuted,
                            unselectedTextColor = colors.onSurfaceMuted,
                        ),
                    )
                }
            }
            Box(modifier = Modifier.weight(1f)) {
                content(PaddingValues())
            }
        }
    } else {
        Column(
            modifier = modifier
                .fillMaxSize()
                .background(colors.background),
        ) {
            Box(modifier = Modifier.weight(1f)) {
                content(PaddingValues(bottom = 0.dp))
            }
            NavigationBar(
                containerColor = colors.surface,
                contentColor = colors.onSurface,
            ) {
                items.forEach { item ->
                    NavigationBarItem(
                        selected = item.destination == selected,
                        onClick = { onSelect(item.destination) },
                        icon = { Icon(item.icon, contentDescription = item.destination.label) },
                        label = { Text(item.destination.label) },
                        colors = NavigationBarItemDefaults.colors(
                            selectedIconColor = colors.onBrand,
                            selectedTextColor = colors.brand,
                            indicatorColor = colors.brand,
                            unselectedIconColor = colors.onSurfaceMuted,
                            unselectedTextColor = colors.onSurfaceMuted,
                        ),
                    )
                }
            }
        }
    }
}

@Composable
private fun TelevisionScaffold(
    selected: TopLevelDestination,
    onSelect: (TopLevelDestination) -> Unit,
    modifier: Modifier,
    content: @Composable (PaddingValues) -> Unit,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    var navFocused by remember { mutableStateOf(false) }
    val expanded = navFocused
    val railWidth = if (expanded) spacing.navExpandedWidth else spacing.navCollapsedWidth

    Row(
        modifier = modifier
            .fillMaxSize()
            .background(colors.background)
            .padding(
                start = spacing.screenHorizontal / 2,
                top = spacing.screenVertical,
                end = spacing.screenHorizontal / 2,
                bottom = spacing.screenVertical,
            ),
    ) {
        Column(
            modifier = Modifier
                .width(railWidth)
                .fillMaxHeight()
                .animateContentSize(animationSpec = PrimerTheme.motion.railExpand)
                .background(colors.surface, PrimerTheme.shapes.panel)
                .padding(vertical = spacing.sm)
                .focusGroup()
                .onFocusChanged { navFocused = it.hasFocus }
                .selectableGroup(),
            verticalArrangement = Arrangement.spacedBy(spacing.sm),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            AllNavItems.forEach { item ->
                TvNavItem(
                    item = item,
                    selected = item.destination == selected,
                    expanded = expanded,
                    onClick = { onSelect(item.destination) },
                )
            }
        }

        Box(
            modifier = Modifier
                .weight(1f)
                .fillMaxHeight()
                .padding(start = spacing.md),
        ) {
            content(PaddingValues())
        }
    }
}

@Composable
private fun TvNavItem(
    item: NavItem,
    selected: Boolean,
    expanded: Boolean,
    onClick: () -> Unit,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing

    TvFocusSurface(
        onClick = onClick,
        selected = selected,
        shape = PrimerTheme.shapes.button,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = spacing.sm),
    ) { focused ->
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = spacing.sm, vertical = spacing.sm),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(spacing.sm),
        ) {
            Icon(
                imageVector = item.icon,
                contentDescription = item.destination.label,
                tint = when {
                    focused || selected -> colors.brand
                    else -> colors.onSurfaceMuted
                },
                modifier = Modifier.size(28.dp),
            )
            if (expanded) {
                Text(
                    text = item.destination.label,
                    style = PrimerTheme.typography.label,
                    color = when {
                        focused || selected -> colors.onSurface
                        else -> colors.onSurfaceMuted
                    },
                )
            }
        }
    }
}
