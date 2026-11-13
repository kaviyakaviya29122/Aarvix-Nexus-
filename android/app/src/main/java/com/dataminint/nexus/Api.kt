package com.dataminint.nexus

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.google.gson.Gson
import com.google.gson.JsonElement
import okhttp3.OkHttpClient
import retrofit2.Response
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query
import java.util.concurrent.TimeUnit

// ---------- models (field names match the Go JSON) ----------
data class Creds(val email: String, val password: String)
data class VerifyReq(val email: String, val code: String)
data class EmailReq(val email: String)
data class LoginResp(val token: String?, val role: String?, val email: String?)
data class Msg(val message: String?, val error: String?)

data class Job(
    val id: String, val type: String, val status: String, val priority: Int,
    val attempts: Int, val max_attempts: Int, val worker_id: String?, val error: String?,
    val created_at: String, val payload: JsonElement?, val result: JsonElement?
)
data class CreateJobReq(
    val type: String, val payload: JsonElement, val priority: Int = 5,
    val max_attempts: Int = 3, val idempotency_key: String? = null
)
data class CreateJobResp(val job_id: String?, val status: String?, val duplicate: Boolean?)
data class WorkerInfo(
    val id: String, val status: String, val capacity: Int, val active_jobs: Long,
    val load: Double, val success_count: Long, val fail_count: Long, val avg_ms: Double
)
data class Stats(
    val jobs_by_status: Map<String, Long>?, val workers_online: Int, val workers_offline: Int,
    val avg_ms: Double, val recovered_attempts: Long, val failed_attempts: Long,
    val duplicates_blocked: Long, val throughput_per_min: Long
)
data class UserRow(val id: String, val email: String, val role: String, val verified: Boolean)

interface NexusApi {
    @POST("api/auth/register") suspend fun register(@Body b: Creds): Response<Msg>
    @POST("api/auth/verify") suspend fun verify(@Body b: VerifyReq): Response<Msg>
    @POST("api/auth/resend") suspend fun resend(@Body b: EmailReq): Response<Msg>
    @POST("api/auth/login") suspend fun login(@Body b: Creds): Response<LoginResp>

    @GET("api/jobs") suspend fun jobs(@Query("status") status: String? = null, @Query("limit") limit: Int = 50): Response<List<Job>>
    @POST("api/jobs") suspend fun createJob(@Body b: CreateJobReq): Response<CreateJobResp>
    @POST("api/jobs/{id}/cancel") suspend fun cancel(@Path("id") id: String): Response<Msg>

    @GET("api/admin/stats") suspend fun stats(): Response<Stats>
    @GET("api/admin/workers") suspend fun workers(): Response<List<WorkerInfo>>
    @GET("api/admin/users") suspend fun users(): Response<List<UserRow>>
    @POST("api/admin/jobs/{id}/retry") suspend fun retry(@Path("id") id: String): Response<Msg>
}

fun <T> Response<T>.errorText(): String = try {
    Gson().fromJson(errorBody()?.string(), Msg::class.java)?.error ?: "HTTP ${code()}"
} catch (e: Exception) { "HTTP ${code()}" }

/** JWT and profile live in EncryptedSharedPreferences (AES-256, key in Android Keystore). */
class TokenStore(ctx: Context) {
    private val prefs = EncryptedSharedPreferences.create(
        ctx, "nexus_secure",
        MasterKey.Builder(ctx).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
    )
    var token: String?
        get() = prefs.getString("token", null)
        set(v) = prefs.edit().putString("token", v).apply()
    var role: String?
        get() = prefs.getString("role", null)
        set(v) = prefs.edit().putString("role", v).apply()
    var email: String?
        get() = prefs.getString("email", null)
        set(v) = prefs.edit().putString("email", v).apply()
    fun clear() = prefs.edit().clear().apply()
}

fun buildApi(store: TokenStore): NexusApi {
    val client = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS).readTimeout(20, TimeUnit.SECONDS)
        .addInterceptor { chain ->
            val b = chain.request().newBuilder()
            store.token?.let { b.header("Authorization", "Bearer $it") }
            chain.proceed(b.build())
        }.build()
    return Retrofit.Builder().baseUrl(BuildConfig.BASE_URL).client(client)
        .addConverterFactory(GsonConverterFactory.create()).build().create(NexusApi::class.java)
}
