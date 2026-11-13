package com.dataminint.nexus

import android.graphics.Color as AndroidColor
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

class MainActivity : ComponentActivity() {
    private val vm: AppViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Transparent status bar with light icons: the gradient header draws underneath it.
        enableEdgeToEdge(statusBarStyle = SystemBarStyle.dark(AndroidColor.TRANSPARENT))
        setContent {
            NexusTheme {
                Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) { Root(vm) }
            }
        }
    }
}

@Composable
fun Root(vm: AppViewModel) {
    when (vm.screen) {
        Screen.LOGIN -> AuthScreen(vm, register = false)
        Screen.REGISTER -> AuthScreen(vm, register = true)
        Screen.VERIFY -> VerifyScreen(vm)
        Screen.HOME -> HomeScreen(vm)
    }
}
