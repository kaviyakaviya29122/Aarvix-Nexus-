package com.dataminint.nexus

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

data class StatusStyle(val color: Color, val icon: ImageVector, val label: String)

fun statusStyle(s: String) = when (s) {
    "succeeded" -> StatusStyle(Brand.Emerald, Icons.Filled.CheckCircle, "Succeeded")
    "running" -> StatusStyle(Brand.Sky, Icons.Filled.PlayArrow, "Running")
    "assigned" -> StatusStyle(Brand.Sky, Icons.Filled.Schedule, "Assigned")
    "pending" -> StatusStyle(Brand.Slate, Icons.Filled.Schedule, "Pending")
    "retrying" -> StatusStyle(Brand.Amber, Icons.Filled.Sync, "Retrying")
    "dead" -> StatusStyle(Brand.Rose, Icons.Filled.Error, "Failed")
    "cancelled" -> StatusStyle(Brand.Slate, Icons.Filled.Cancel, "Cancelled")
    else -> StatusStyle(Brand.Slate, Icons.Filled.Info, s)
}

@Composable
fun StatusChip(status: String) {
    val st = statusStyle(status)
    Row(
        Modifier.clip(RoundedCornerShape(50)).background(st.color.copy(alpha = 0.14f))
            .padding(horizontal = 10.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(st.icon, null, tint = st.color, modifier = Modifier.size(14.dp))
        Spacer(Modifier.width(4.dp))
        Text(st.label, color = st.color, style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.Bold)
    }
}

@Composable
fun GradientHeader(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier.fillMaxWidth()
            .clip(RoundedCornerShape(bottomStart = 28.dp, bottomEnd = 28.dp))
            .background(Brand.Gradient)
            .statusBarsPadding()
            .padding(horizontal = 20.dp, vertical = 16.dp),
        content = content
    )
}

@Composable
fun MessageBar(vm: AppViewModel) {
    if (vm.busy) LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 8.dp).clip(RoundedCornerShape(50)))
    vm.message?.let { m ->
        val c = if (vm.isError) Brand.Rose else Brand.Indigo
        Row(
            Modifier.fillMaxWidth().padding(top = 8.dp).clip(RoundedCornerShape(12.dp))
                .background(c.copy(alpha = 0.12f)).padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(if (vm.isError) Icons.Filled.Warning else Icons.Filled.Info, null, tint = c, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(m, color = c, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

/** Gradient top + white rounded sheet: shared by login, register and verification screens. */
@Composable
fun AuthShell(icon: ImageVector, content: @Composable ColumnScope.() -> Unit) {
    Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface)) {
        Box(Modifier.fillMaxWidth().height(320.dp).background(Brand.Gradient))
        Column(Modifier.fillMaxSize().statusBarsPadding().imePadding().verticalScroll(rememberScrollState())) {
            Column(
                Modifier.fillMaxWidth().padding(top = 36.dp, bottom = 28.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Box(
                    Modifier.size(76.dp).clip(CircleShape).background(Color.White.copy(alpha = 0.2f)),
                    contentAlignment = Alignment.Center
                ) { Icon(icon, null, tint = Color.White, modifier = Modifier.size(40.dp)) }
                Spacer(Modifier.height(14.dp))
                Text("AARVIX NEXUS", color = Color.White, fontSize = 26.sp, fontWeight = FontWeight.ExtraBold, letterSpacing = 3.sp)
                Text("Self-Healing Distributed Cloud", color = Color.White.copy(alpha = 0.85f), style = MaterialTheme.typography.bodyMedium)
            }
            Surface(
                Modifier.fillMaxWidth(), shape = RoundedCornerShape(topStart = 32.dp, topEnd = 32.dp),
                color = MaterialTheme.colorScheme.surface, shadowElevation = 12.dp
            ) {
                Column(Modifier.padding(24.dp).navigationBarsPadding(), content = content)
            }
        }
    }
}

@Composable
fun Rule(ok: Boolean, text: String) {
    Row(Modifier.padding(top = 2.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(
            if (ok) Icons.Filled.CheckCircle else Icons.Filled.RadioButtonUnchecked, null,
            tint = if (ok) Brand.Emerald else MaterialTheme.colorScheme.outline, modifier = Modifier.size(16.dp)
        )
        Spacer(Modifier.width(6.dp))
        Text(text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

data class StatItem(val label: String, val value: String, val icon: ImageVector, val a: Color, val b: Color)

@Composable
fun StatGrid(items: List<StatItem>) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        items.chunked(2).forEach { row ->
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                row.forEach { StatCard(it, Modifier.weight(1f)) }
                if (row.size == 1) Spacer(Modifier.weight(1f))
            }
        }
    }
}

@Composable
fun StatCard(s: StatItem, modifier: Modifier = Modifier) {
    Box(modifier.clip(RoundedCornerShape(18.dp)).background(Brand.grad(s.a, s.b)).padding(14.dp)) {
        Column {
            Icon(s.icon, null, tint = Color.White.copy(alpha = 0.9f), modifier = Modifier.size(22.dp))
            Spacer(Modifier.height(10.dp))
            Text(s.value, color = Color.White, fontSize = 24.sp, fontWeight = FontWeight.ExtraBold)
            Text(s.label, color = Color.White.copy(alpha = 0.9f), style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
fun MiniStat(label: String, value: String, modifier: Modifier = Modifier) {
    Column(
        modifier.clip(RoundedCornerShape(14.dp)).background(Color.White.copy(alpha = 0.18f)).padding(vertical = 10.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text(value, color = Color.White, fontSize = 20.sp, fontWeight = FontWeight.ExtraBold)
        Text(label, color = Color.White.copy(alpha = 0.9f), style = MaterialTheme.typography.labelSmall)
    }
}

@Composable
fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(text, style = MaterialTheme.typography.titleMedium, modifier = modifier.padding(top = 8.dp))
}

@Composable
fun EmptyState(icon: ImageVector, title: String, hint: String) {
    Column(Modifier.fillMaxWidth().padding(vertical = 48.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Box(
            Modifier.size(72.dp).clip(CircleShape).background(MaterialTheme.colorScheme.primaryContainer),
            contentAlignment = Alignment.Center
        ) { Icon(icon, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(34.dp)) }
        Spacer(Modifier.height(12.dp))
        Text(title, style = MaterialTheme.typography.titleMedium)
        Text(hint, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
