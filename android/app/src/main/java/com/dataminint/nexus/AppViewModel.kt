package com.dataminint.nexus

import android.app.Application
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.google.gson.JsonParser
import kotlinx.coroutines.launch
import retrofit2.Response
import java.util.UUID

enum class Screen { LOGIN, REGISTER, VERIFY, HOME }

class AppViewModel(app: Application) : AndroidViewModel(app) {
    private val store = TokenStore(app)
    private val api = buildApi(store)

    var screen by mutableStateOf(if (store.token != null) Screen.HOME else Screen.LOGIN)
    var email by mutableStateOf(store.email ?: "")
    var role by mutableStateOf(store.role ?: "user")
    var busy by mutableStateOf(false)
    var message by mutableStateOf<String?>(null)
    var isError by mutableStateOf(false)

    var jobs by mutableStateOf<List<Job>>(emptyList())
    var stats by mutableStateOf<Stats?>(null)
    var workers by mutableStateOf<List<WorkerInfo>>(emptyList())
    var problemJobs by mutableStateOf<List<Job>>(emptyList())   // retrying + dead
    var users by mutableStateOf<List<UserRow>>(emptyList())

    val isAdmin get() = role == "admin"

    /** Runs a call; on 401 the session is dropped and the login screen is shown. */
    private fun <T> call(
        show: Boolean = true,
        onFail: (Int, String) -> Boolean = { _, _ -> false },
        block: suspend () -> Response<T>,
        ok: (T) -> Unit
    ) {
        viewModelScope.launch {
            if (show) busy = true
            isError = false
            try {
                val r = block()
                when {
                    r.isSuccessful -> r.body()?.let(ok)
                    r.code() == 401 && store.token != null -> logout("Session expired, please log in again")
                    else -> {
                        val t = r.errorText()
                        if (!onFail(r.code(), t)) { message = t; isError = true }
                    }
                }
            } catch (e: Exception) {
                message = "Cannot reach server"; isError = true
                android.util.Log.e("NEXUS", "request failed", e)
            } finally {
                if (show) busy = false
            }
        }
    }

    fun login(mail: String, password: String) {
        email = mail.trim().lowercase()
        call(
            // an unverified account answers 403 "email_not_verified" -> send a fresh code and go to verification
            onFail = { code, text ->
                if (code == 403 && text == "email_not_verified") {
                    screen = Screen.VERIFY; message = "Please verify your email first"; resend(); true
                } else false
            },
            block = { api.login(Creds(email, password)) },
            ok = {
                store.token = it.token; store.role = it.role; store.email = it.email
                role = it.role ?: "user"; screen = Screen.HOME; message = null
            })
    }

    fun register(mail: String, password: String) {
        email = mail.trim().lowercase()
        call(block = { api.register(Creds(email, password)) }, ok = {
            screen = Screen.VERIFY; message = "Verification code sent to $email"
        })
    }

    fun verify(code: String) = call(block = { api.verify(VerifyReq(email, code.trim())) }, ok = {
        screen = Screen.LOGIN; message = "Email verified. Please log in."
    })

    fun resend() = call(block = { api.resend(EmailReq(email)) }, ok = { message = "New code sent" })

    fun logout(note: String? = null) {
        store.clear(); jobs = emptyList(); stats = null; workers = emptyList()
        problemJobs = emptyList(); users = emptyList()
        screen = Screen.LOGIN; message = note; isError = note != null
    }

    fun go(s: Screen) { screen = s; message = null; isError = false }

    fun loadJobs(quiet: Boolean = false) = call(show = !quiet, block = { api.jobs() }, ok = { jobs = it })
    private fun stableKey(type: String, payloadJson: String, priority: Int): String {
        val normalized = JsonParser.parseString(payloadJson).toString()
        return java.security.MessageDigest.getInstance("SHA-256")
            .digest("$type|$normalized|$priority".toByteArray())
            .joinToString("") { "%02x".format(it) }
    }

    fun createJob(type: String, payloadJson: String, priority: Int, maxAttempts: Int, onDone: () -> Unit) {
        val payload = try { JsonParser.parseString(payloadJson) } catch (e: Exception) {
            message = "Payload is not valid JSON"; isError = true; return
        }
        call(block = {
            api.createJob(CreateJobReq(type, payload, priority, maxAttempts, stableKey(type, payloadJson, priority)))
        }, ok = {
            message = if (it.duplicate == true) "Duplicate detected: existing job ${it.job_id?.take(8)} returned"
            else "Job ${it.job_id?.take(8)} queued"
            loadJobs(true); onDone()
        })
    }

    fun cancel(id: String) = call(block = { api.cancel(id) }, ok = { loadJobs(true) })

    fun loadAdmin(quiet: Boolean = false) {
        call(show = !quiet, block = { api.stats() }, ok = { stats = it })
        call(show = false, block = { api.workers() }, ok = { workers = it })
        call(show = false, block = { api.users() }, ok = { users = it })
        call(show = false, block = { api.jobs("dead", 100) }, ok = { dead ->
            call(show = false, block = { api.jobs("retrying", 100) }, ok = { retrying -> problemJobs = dead + retrying })
        })
    }

    fun retry(id: String) = call(block = { api.retry(id) }, ok = { loadAdmin(true) })
}
