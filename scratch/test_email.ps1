# Email connectivity test script
# Usage:
#   $env:MAIL_HOST="polygraph.ae"
#   $env:MAIL_USER="admin@polygraph.ae"
#   $env:MAIL_PASS="<password>"
#   pwsh -File scratch/test_email.ps1
#
# Or pass params directly:
#   pwsh -File scratch/test_email.ps1 -Host polygraph.ae -User admin@polygraph.ae

param(
    [string]$MailHost     = $env:MAIL_HOST,
    [string]$User     = $env:MAIL_USER,
    [string]$Pass     = $env:MAIL_PASS,
    [string]$ImapPortStr = $env:IMAP_PORT,
    [string]$SmtpPortStr = $env:SMTP_PORT
)

$ImapPort = if ($ImapPortStr) { [int]$ImapPortStr } else { 993 }
$SmtpPort = if ($SmtpPortStr) { [int]$SmtpPortStr } else { 465 }

if (-not $MailHost -or -not $User -or -not $Pass) {
    Write-Error "MAIL_HOST, MAIL_USER, and MAIL_PASS must be set (env vars or -Host/-User/-Pass params)"
    exit 1
}

Write-Host "=== Email Connectivity Test ===" -ForegroundColor Cyan
Write-Host "Host : $MailHost"
Write-Host "User : $User"
Write-Host ""

$failed = $false

# ---- Helper: open TLS stream ----
function Open-TlsStream([string]$server, [int]$port) {
    $tcp = New-Object System.Net.Sockets.TcpClient
    $tcp.Connect($server, $port)
    $ssl = New-Object System.Net.Security.SslStream($tcp.GetStream(), $false,
        { param($s,$cert,$chain,$err) $true })   # accept server cert
    $ssl.AuthenticateAsClient($server)
    return @{ tcp = $tcp; ssl = $ssl }
}

function Close-Stream($conn) {
    try { $conn.ssl.Close() } catch {}
    try { $conn.tcp.Close() } catch {}
}

function Read-Line($stream, [int]$timeoutMs = 8000) {
    $buf = New-Object byte[] 4096
    $sb  = New-Object System.Text.StringBuilder
    $deadline = (Get-Date).AddMilliseconds($timeoutMs)
    while ((Get-Date) -lt $deadline) {
        if ($stream.DataAvailable -or $stream.ssl.CanRead) {
            $n = $stream.ssl.Read($buf, 0, $buf.Length)
            $sb.Append([System.Text.Encoding]::ASCII.GetString($buf, 0, $n)) | Out-Null
            $s = $sb.ToString()
            if ($s.Contains("`n")) { return $s.Trim() }
        }
        Start-Sleep -Milliseconds 50
    }
    return $sb.ToString().Trim()
}

function Write-Line($ssl, [string]$line) {
    $bytes = [System.Text.Encoding]::ASCII.GetBytes("$line`r`n")
    $ssl.Write($bytes, 0, $bytes.Length)
}

# ---- IMAP Test ----
Write-Host "[IMAP] Connecting to ${Host}:${ImapPort} ..." -ForegroundColor Yellow
try {
    $conn = Open-TlsStream $MailHost $ImapPort
    Write-Host "[IMAP] TLS handshake OK" -ForegroundColor Green

    # Read greeting
    $greeting = Read-Line $conn
    Write-Host "[IMAP] Greeting: $greeting"

    # Send LOGIN
    Write-Line $conn.ssl "a001 LOGIN `"$User`" `"$Pass`""
    $resp = Read-Line $conn
    Write-Host "[IMAP] LOGIN response: $resp"

    # Logout
    Write-Line $conn.ssl "a002 LOGOUT"
    Close-Stream $conn

    if ($resp -match "a001 OK") {
        Write-Host "[IMAP] PASSED" -ForegroundColor Green
    } else {
        Write-Host "[IMAP] FAILED - unexpected response" -ForegroundColor Red
        $failed = $true
    }
}
catch {
    Write-Host "[IMAP] FAILED: $_" -ForegroundColor Red
    $failed = $true
}

Write-Host ""

# ---- SMTP Test (implicit TLS / SMTPS on 465) ----
Write-Host "[SMTP] Connecting to ${Host}:${SmtpPort} ..." -ForegroundColor Yellow
try {
    $conn = Open-TlsStream $MailHost $SmtpPort
    Write-Host "[SMTP] TLS handshake OK" -ForegroundColor Green

    # Read banner
    $banner = Read-Line $conn
    Write-Host "[SMTP] Banner: $banner"

    # EHLO
    Write-Line $conn.ssl "EHLO testclient"
    $ehlo = Read-Line $conn
    Write-Host "[SMTP] EHLO response: $(($ehlo -split "`n" | Select-Object -First 3) -join ' | ')"

    # AUTH LOGIN
    Write-Line $conn.ssl "AUTH LOGIN"
    $auth1 = Read-Line $conn  # expects "334 VXNlcm5hbWU6" (Username:)
    Write-Host "[SMTP] AUTH prompt: $auth1"

    $userB64 = [Convert]::ToBase64String([System.Text.Encoding]::ASCII.GetBytes($User))
    Write-Line $conn.ssl $userB64
    $auth2 = Read-Line $conn  # expects "334 UGFzc3dvcmQ6" (Password:)
    Write-Host "[SMTP] Password prompt: $auth2"

    $passB64 = [Convert]::ToBase64String([System.Text.Encoding]::ASCII.GetBytes($Pass))
    Write-Line $conn.ssl $passB64
    $auth3 = Read-Line $conn  # expects "235 2.7.0 Authentication successful"
    Write-Host "[SMTP] AUTH response: $auth3"

    # QUIT
    Write-Line $conn.ssl "QUIT"
    Close-Stream $conn

    if ($auth3 -match "^235") {
        Write-Host "[SMTP] PASSED" -ForegroundColor Green
    } else {
        Write-Host "[SMTP] FAILED - unexpected auth response" -ForegroundColor Red
        $failed = $true
    }
}
catch {
    Write-Host "[SMTP] FAILED: $_" -ForegroundColor Red
    $failed = $true
}

Write-Host ""
if ($failed) {
    Write-Host "Result: one or more checks FAILED" -ForegroundColor Red
    exit 1
} else {
    Write-Host "Result: all checks PASSED - safe to configure in production" -ForegroundColor Green
    exit 0
}

