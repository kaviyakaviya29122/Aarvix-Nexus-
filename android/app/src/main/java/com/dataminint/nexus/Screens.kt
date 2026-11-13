@file:OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)

package com.dataminint.nexus

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay

private val Field = RoundedCornerShape(14.dp)

// ============================================================ authentication

@Composable
fun AuthScreen(vm: AppViewModel, register: Boolean) {
    var email by remember { mutableStateOf(vm.email) }
    var pass by remember { mutableStateOf("") }
    var show by remember { mutableStateOf(false) }

    AuthShell(Icons.Filled.Hub) {
        Text(if (register) "Create your account" else "Welcome back", style = MaterialTheme.typography.headlineSmall)
        Text(
            if (register) "Register, then verify your email to get started." else "Sign in to manage your jobs and workers.",
            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Spacer(Modifier.height(20.dp))
        OutlinedTextField(
            email, { email = it }, label = { Text("Email") }, singleLine = true, shape = Field,
            leadingIcon = { Icon(Icons.Filled.Email, null) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email), modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(
            pass, { pass = it }, label = { Text("Password") }, singleLine = true, shape = Field,
            leadingIcon = { Icon(Icons.Filled.Lock, null) },
            trailingIcon = {
                IconButton(onClick = { show = !show }) {
                    Icon(if (show) Icons.Filled.VisibilityOff else Icons.Filled.Visibility, "Toggle password")
                }
            },
            visualTransformation = if (show) VisualTransformation.None else PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password), modifier = Modifier.fillMaxWidth()
        )
        if (register) {
            Spacer(Modifier.height(8.dp))
            Rule(pass.length in 10..72, "10 or more characters")
            Rule(pass.any(Char::isLetter), "Contains a letter")
            Rule(pass.any(Char::isDigit), "Contains a digit")
        }
        Spacer(Modifier.height(20.dp))
        Button(
            onClick = { if (register) vm.register(email, pass) else vm.login(email, pass) },
            enabled = !vm.busy && email.isNotBlank() && pass.isNotEmpty(),
            modifier = Modifier.fillMaxWidth().height(52.dp), shape = Field
        ) { Text(if (register) "Create account" else "Sign in", fontSize = 16.sp) }
        MessageBar(vm)
        Spacer(Modifier.height(8.dp))
        TextButton(onClick = { vm.go(if (register) Screen.LOGIN else Screen.REGISTER) }, modifier = Modifier.align(Alignment.CenterHorizontally)) {
            Text(if (register) "Already have an account? Sign in" else "New here? Create an account")
        }
    }
}

@Composable
fun VerifyScreen(vm: AppViewModel) {
    var code by remember { mutableStateOf("") }
    AuthShell(Icons.Filled.MarkEmailRead) {
        Text("Verify your email", style = MaterialTheme.typography.headlineSmall)
        Text(
            "Enter the 6-digit code sent to ${vm.email}. It expires in 15 minutes.",
            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Spacer(Modifier.height(20.dp))
        OutlinedTextField(
            code, { if (it.length <= 6 && it.all(Char::isDigit)) code = it },
            placeholder = { Text("000000", Modifier.fillMaxWidth(), textAlign = TextAlign.Center, fontSize = 28.sp, letterSpacing = 8.sp) },
            textStyle = TextStyle(fontSize = 28.sp, letterSpacing = 8.sp, textAlign = TextAlign.Center, fontWeight = FontWeight.Bold),
            singleLine = true, shape = Field,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword), modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(20.dp))
        Button(
            onClick = { vm.verify(code) }, enabled = !vm.busy && code.length == 6,
            modifier = Modifier.fillMaxWidth().height(52.dp), shape = Field
        ) { Text("Verify email", fontSize = 16.sp) }
        MessageBar(vm)
        Row(Modifier.align(Alignment.CenterHorizontally)) {
            TextButton(onClick = { vm.resend() }, enabled = !vm.busy) { Text("Resend code") }
            TextButton(onClick = { vm.go(Screen.LOGIN) }) { Text("Back to login") }
        }
    }
}

// ============================================================ home

@Composable
fun HomeScreen(vm: AppViewModel) {
    var tab by remember { mutableIntStateOf(0) }
    val titles = if (vm.isAdmin) listOf("My jobs", "New job", "Admin") else listOf("My jobs", "New job")
    val icons = listOf(Icons.AutoMirrored.Filled.List, Icons.Filled.AddCircle, Icons.Filled.AdminPanelSettings)
    val heading = listOf("Your jobs", "Create a job", "Admin console")[tab]

    Scaffold(
        containerColor = MaterialTheme.colorScheme.background,
        bottomBar = {
            NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                titles.forEachIndexed { i, t ->
                    NavigationBarItem(
                        selected = tab == i, onClick = { tab = i; vm.go(Screen.HOME) },
                        icon = { Icon(icons[i], t) }, label = { Text(t) },
                        colors = NavigationBarItemDefaults.colors(indicatorColor = MaterialTheme.colorScheme.primaryContainer)
                    )
                }
            }
        }
    ) { pad ->
        Column(Modifier.padding(bottom = pad.calculateBottomPadding()).fillMaxSize()) {
            GradientHeader {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(heading, color = Color.White, style = MaterialTheme.typography.headlineSmall)
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(vm.email, color = Color.White.copy(alpha = 0.85f), style = MaterialTheme.typography.bodySmall, maxLines = 1)
                            if (vm.isAdmin) {
                                Spacer(Modifier.width(8.dp))
                                Text(
                                    "ADMIN", color = Color.White, fontSize = 10.sp, fontWeight = FontWeight.Bold,
                                    modifier = Modifier.clip(RoundedCornerShape(50)).background(Color.White.copy(alpha = 0.25f))
                                        .padding(horizontal = 8.dp, vertical = 2.dp)
                                )
                            }
                        }
                    }
                    IconButton(onClick = { vm.logout() }) {
                        Icon(Icons.AutoMirrored.Filled.Logout, "Logout", tint = Color.White)
                    }
                }
                if (tab == 0) {
                    Spacer(Modifier.height(14.dp))
                    val active = vm.jobs.count { it.status in ACTIVE }
                    val done = vm.jobs.count { it.status == "succeeded" }
                    val failed = vm.jobs.count { it.status in FAILED }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        MiniStat("Total", vm.jobs.size.toString(), Modifier.weight(1f))
                        MiniStat("Active", active.toString(), Modifier.weight(1f))
                        MiniStat("Done", done.toString(), Modifier.weight(1f))
                        MiniStat("Failed", failed.toString(), Modifier.weight(1f))
                    }
                }
            }
            Column(Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                MessageBar(vm)
                when (tab) {
                    0 -> JobsTab(vm)
                    1 -> NewJobTab(vm) { tab = 0 }
                    else -> AdminTab(vm)
                }
            }
        }
    }
}

private val ACTIVE = listOf("pending", "assigned", "running", "retrying")
private val FAILED = listOf("dead", "retrying")

// ============================================================ jobs

@Composable
fun JobsTab(vm: AppViewModel) {
    LaunchedEffect(Unit) {            // live refresh every 5s while this tab is visible
        vm.loadJobs()
        while (true) { delay(5000); vm.loadJobs(quiet = true) }
    }
    var filter by remember { mutableStateOf("All") }
    val shown = vm.jobs.filter {
        when (filter) {
            "Active" -> it.status in listOf("pending", "assigned", "running")
            "Done" -> it.status == "succeeded"
            "Failed" -> it.status in FAILED
            else -> true
        }
    }
    Row(Modifier.padding(vertical = 10.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        listOf("All", "Active", "Done", "Failed").forEach { f ->
            FilterChip(selected = filter == f, onClick = { filter = f }, label = { Text(f) }, shape = RoundedCornerShape(50))
        }
    }
    LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp), contentPadding = PaddingValues(bottom = 16.dp)) {
        if (shown.isEmpty()) item { EmptyState(Icons.Filled.Inbox, "No jobs here", "Create a job from the New job tab.") }
        items(shown, key = { it.id }) { j ->
            JobCard(j) {
                if (j.status in listOf("pending", "retrying", "assigned", "running")) {
                    OutlinedButton(onClick = { vm.cancel(j.id) }, shape = RoundedCornerShape(12.dp), modifier = Modifier.padding(top = 8.dp)) {
                        Icon(Icons.Filled.Cancel, null, Modifier.size(16.dp)); Spacer(Modifier.width(6.dp)); Text("Cancel")
                    }
                }
            }
        }
    }
}

@Composable
fun JobCard(j: Job, actions: @Composable ColumnScope.() -> Unit = {}) {
    val st = statusStyle(j.status)
    Card(
        Modifier.fillMaxWidth(), shape = RoundedCornerShape(18.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Row(Modifier.height(IntrinsicSize.Min)) {
            Box(Modifier.width(6.dp).fillMaxHeight().background(st.color))
            Column(Modifier.padding(14.dp).weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        j.type.replace('_', ' ').replaceFirstChar { it.uppercase() },
                        style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f)
                    )
                    StatusChip(j.status)
                }
                Text(
                    "#${j.id.take(8)}  ·  attempt ${j.attempts}/${j.max_attempts}" + (j.worker_id?.let { "  ·  $it" } ?: ""),
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                if (j.status == "running" || j.status == "assigned") {
                    LinearProgressIndicator(
                        color = st.color, modifier = Modifier.fillMaxWidth().padding(top = 10.dp).clip(RoundedCornerShape(50))
                    )
                }
                j.error?.let {
                    Text(
                        it, color = Brand.Rose, style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.padding(top = 8.dp).fillMaxWidth().clip(RoundedCornerShape(10.dp))
                            .background(Brand.Rose.copy(alpha = 0.10f)).padding(8.dp)
                    )
                }
                j.result?.takeIf { !it.isJsonNull }?.let {
                    Text(
                        it.toString(), fontFamily = FontFamily.Monospace, fontSize = 12.sp,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 8.dp).fillMaxWidth().clip(RoundedCornerShape(10.dp))
                            .background(MaterialTheme.colorScheme.surfaceVariant).padding(8.dp)
                    )
                }
                actions()
            }
        }
    }
}

@Composable
fun NewJobTab(vm: AppViewModel, onCreated: () -> Unit) {
    val types = remember { listOf("echo", "sleep", "compute", "business_op") + if (vm.isAdmin) listOf("flaky", "fail") else emptyList() }
    val templates = mapOf(
        "echo" to """{"hello":"world"}""",
        "sleep" to """{"seconds":5}""",
        "compute" to """{"n":1000000}""",
        "business_op" to """{"account":"ACC-1","amount_cents":2500}""",
        "flaky" to """{"fail_times":2}""",
        "fail" to "{}"
    )
    val about = mapOf(
        "echo" to "Returns the payload. Handy for a quick connectivity check.",
        "sleep" to "Simulates long-running work for the given number of seconds.",
        "compute" to "CPU task: counts the prime numbers below n.",
        "business_op" to "Applies a ledger operation exactly once, even if the job is retried.",
        "flaky" to "Admin test: fails N times, then succeeds (shows automatic retry).",
        "fail" to "Admin test: permanent failure (shows the failed state and admin retry)."
    )
    var type by remember { mutableStateOf("echo") }
    var payload by remember { mutableStateOf(templates.getValue("echo")) }
    var priority by remember { mutableFloatStateOf(5f) }

    Column(Modifier.verticalScroll(rememberScrollState()).padding(top = 12.dp, bottom = 16.dp)) {
        Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(20.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
            Column(Modifier.padding(16.dp)) {
                Text("Job type", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(8.dp))
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    types.forEach { t ->
                        FilterChip(
                            selected = type == t, onClick = { type = t; payload = templates.getValue(t) },
                            label = { Text(t) }, shape = RoundedCornerShape(50)
                        )
                    }
                }
                Text(
                    about.getValue(type), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 8.dp)
                )
                Spacer(Modifier.height(16.dp))
                OutlinedTextField(
                    payload, { payload = it }, label = { Text("Payload (JSON)") }, shape = Field,
                    textStyle = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 14.sp),
                    modifier = Modifier.fillMaxWidth(), minLines = 3
                )
                Spacer(Modifier.height(12.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Priority", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                    Text(
                        priority.toInt().toString(), color = Color.White, fontWeight = FontWeight.Bold,
                        modifier = Modifier.clip(CircleShape).background(Brand.Gradient).padding(horizontal = 12.dp, vertical = 4.dp)
                    )
                }
                Slider(priority, { priority = it }, valueRange = 1f..10f, steps = 8)
                Spacer(Modifier.height(8.dp))
                Button(
                    onClick = { vm.createJob(type, payload, priority.toInt(), 3, onCreated) }, enabled = !vm.busy,
                    modifier = Modifier.fillMaxWidth().height(52.dp), shape = Field
                ) {
                    Icon(Icons.AutoMirrored.Filled.Send, null, Modifier.size(18.dp)); Spacer(Modifier.width(8.dp))
                    Text("Submit job", fontSize = 16.sp)
                }
            }
        }
        Row(
            Modifier.padding(top = 12.dp).fillMaxWidth().clip(RoundedCornerShape(14.dp))
                .background(Brand.Indigo.copy(alpha = 0.10f)).padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(Icons.Filled.ContentCopy, null, tint = Brand.Indigo, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(
                "Duplicate protection is on: submitting the same payload again returns the existing job instead of running it twice.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

// ============================================================ admin panel

@Composable
fun AdminTab(vm: AppViewModel) {
    LaunchedEffect(Unit) {
        vm.loadAdmin()
        while (true) { delay(5000); vm.loadAdmin(quiet = true) }
    }
    LazyColumn(
        verticalArrangement = Arrangement.spacedBy(10.dp),
        contentPadding = PaddingValues(top = 12.dp, bottom = 16.dp)
    ) {
        item {
            val s = vm.stats
            if (s == null) {
                EmptyState(Icons.Filled.Dns, "Loading system data", "Fetching live statistics…")
            } else {
                val by = s.jobs_by_status ?: emptyMap()
                StatGrid(
                    listOf(
                        StatItem("Total jobs", by.values.sum().toString(), Icons.Filled.Inbox, Brand.Indigo, Brand.Violet),
                        StatItem("Workers online", s.workers_online.toString(), Icons.Filled.Dns, Brand.Emerald, Brand.Teal),
                        StatItem(
                            "Workers offline", s.workers_offline.toString(), Icons.Filled.Warning,
                            if (s.workers_offline > 0) Brand.Rose else Brand.Slate,
                            if (s.workers_offline > 0) Brand.Orange else Brand.SlateDark
                        ),
                        StatItem("Succeeded", (by["succeeded"] ?: 0).toString(), Icons.Filled.CheckCircle, Brand.Green, Brand.Emerald),
                        StatItem("Running", ((by["running"] ?: 0) + (by["assigned"] ?: 0)).toString(), Icons.Filled.PlayArrow, Brand.Sky, Brand.Indigo),
                        StatItem("Retrying", (by["retrying"] ?: 0).toString(), Icons.Filled.Sync, Brand.Amber, Brand.Orange),
                        StatItem("Failed (dead)", (by["dead"] ?: 0).toString(), Icons.Filled.Error, Brand.Rose, Brand.Crimson),
                        StatItem("Avg time", "${s.avg_ms.toInt()} ms", Icons.Filled.Timer, Brand.Violet, Brand.Fuchsia),
                        StatItem("Done / min", s.throughput_per_min.toString(), Icons.Filled.Speed, Brand.Cyan, Brand.Sky),
                        StatItem("Auto-recovered", s.recovered_attempts.toString(), Icons.Filled.Replay, Brand.Teal, Brand.Cyan),
                        StatItem("Failed attempts", s.failed_attempts.toString(), Icons.Filled.Warning, Brand.Orange, Brand.Rose),
                        StatItem("Duplicates blocked", s.duplicates_blocked.toString(), Icons.Filled.ContentCopy, Brand.SlateDark, Brand.Indigo)
                    )
                )
            }
        }

        item { SectionTitle("Workers") }
        if (vm.workers.isEmpty()) item { Text("No workers registered yet.", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        items(vm.workers, key = { it.id }) { w -> WorkerCard(w) }

        item { SectionTitle("Failed and retrying jobs") }
        if (vm.problemJobs.isEmpty()) {
            item { EmptyState(Icons.Filled.CheckCircle, "All clear", "No failed or retrying jobs right now.") }
        }
        items(vm.problemJobs, key = { "p" + it.id }) { j ->
            JobCard(j) {
                if (j.status == "dead") {
                    Button(
                        onClick = { vm.retry(j.id) }, shape = RoundedCornerShape(12.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = Brand.Indigo),
                        modifier = Modifier.padding(top = 8.dp)
                    ) { Icon(Icons.Filled.Replay, null, Modifier.size(16.dp)); Spacer(Modifier.width(6.dp)); Text("Retry job") }
                } else {
                    Text(
                        "Automatic retry scheduled", style = MaterialTheme.typography.bodySmall,
                        color = Brand.Amber, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 8.dp)
                    )
                }
            }
        }

        item { SectionTitle("Users") }
        items(vm.users, key = { "u" + it.id }) { u -> UserRowView(u) }
    }
}

@Composable
fun WorkerCard(w: WorkerInfo) {
    val online = w.status == "online"
    val c = if (online) Brand.Emerald else Brand.Rose
    Card(
        Modifier.fillMaxWidth(), shape = RoundedCornerShape(18.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(2.dp)
    ) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(10.dp).clip(CircleShape).background(c))
                Spacer(Modifier.width(8.dp))
                Icon(Icons.Filled.Memory, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(20.dp))
                Spacer(Modifier.width(6.dp))
                Text(w.id, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                Text(
                    w.status.uppercase(), color = c, fontWeight = FontWeight.Bold, fontSize = 11.sp,
                    modifier = Modifier.clip(RoundedCornerShape(50)).background(c.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 4.dp)
                )
            }
            Spacer(Modifier.height(10.dp))
            Meter("Jobs ${w.active_jobs}/${w.capacity}", (w.active_jobs.toFloat() / w.capacity.coerceAtLeast(1)).coerceIn(0f, 1f), Brand.Indigo)
            Spacer(Modifier.height(8.dp))
            Meter("CPU load ${"%.0f".format(w.load * 100)}%", w.load.toFloat().coerceIn(0f, 1f), if (w.load > 0.8) Brand.Rose else Brand.Cyan)
            Spacer(Modifier.height(8.dp))
            Row {
                Text("✓ ${w.success_count} ok", color = Brand.Emerald, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.width(14.dp))
                Text("✗ ${w.fail_count} failed", color = Brand.Rose, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.width(14.dp))
                Text("avg ${w.avg_ms.toInt()} ms", color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

@Composable
fun Meter(label: String, fraction: Float, color: Color) {
    Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    LinearProgressIndicator(
        progress = { fraction }, color = color, trackColor = color.copy(alpha = 0.15f),
        modifier = Modifier.fillMaxWidth().padding(top = 3.dp).height(7.dp).clip(RoundedCornerShape(50))
    )
}

@Composable
fun UserRowView(u: UserRow) {
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.surface).padding(12.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Box(Modifier.size(40.dp).clip(CircleShape).background(Brand.Gradient), contentAlignment = Alignment.Center) {
            Text(u.email.take(1).uppercase(), color = Color.White, fontWeight = FontWeight.Bold)
        }
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(u.email, style = MaterialTheme.typography.bodyMedium, maxLines = 1)
            Text(
                if (u.verified) "Verified" else "Awaiting verification", style = MaterialTheme.typography.bodySmall,
                color = if (u.verified) Brand.Emerald else Brand.Amber
            )
        }
        val rc = if (u.role == "admin") Brand.Violet else Brand.Slate
        Text(
            u.role.uppercase(), color = rc, fontSize = 11.sp, fontWeight = FontWeight.Bold,
            modifier = Modifier.clip(RoundedCornerShape(50)).background(rc.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 4.dp)
        )
    }
}
