# Runs once at first logon of the test VM (see autounattend.xml).
# Installs WinFsp, enables sshd with the host's key, trims the image,
# and powers off.

$ErrorActionPreference = 'Continue'
$src = Split-Path -Parent $MyInvocation.MyCommand.Path
Start-Transcript -Path C:\gofuse-setup.log

New-Item -ItemType Directory -Force C:\gofuse | Out-Null

Write-Host 'installing WinFsp'
$msi = Get-ChildItem $src -Filter 'winfsp-*.msi' | Select-Object -First 1
Start-Process msiexec.exe -Wait -ArgumentList '/i', $msi.FullName, '/qn', 'INSTALLLEVEL=1000', '/l*v', 'C:\gofuse-winfsp.log'

Write-Host 'enabling sshd'
if (-not (Get-Service sshd -ErrorAction SilentlyContinue)) {
    Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
}
Set-Service sshd -StartupType Automatic
Start-Service sshd
$keys = 'C:\ProgramData\ssh\administrators_authorized_keys'
Copy-Item (Join-Path $src 'authorized_keys') $keys
icacls.exe $keys /inheritance:r /grant 'Administrators:F' /grant 'SYSTEM:F'
New-NetFirewallRule -Name gofuse-sshd -DisplayName 'sshd' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 -ErrorAction SilentlyContinue
New-ItemProperty -Path 'HKLM:\SOFTWARE\OpenSSH' -Name DefaultShell -Value 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' -PropertyType String -Force

Write-Host 'trimming'
Set-MpPreference -DisableRealtimeMonitoring $true -ErrorAction SilentlyContinue
powercfg.exe /hibernate off
$cs = Get-CimInstance Win32_ComputerSystem
Set-CimInstance -InputObject $cs -Property @{AutomaticManagedPagefile = $false}
Get-CimInstance Win32_PageFileSetting | Set-CimInstance -Property @{InitialSize = 256; MaximumSize = 1024}
Stop-Service wuauserv -ErrorAction SilentlyContinue
Set-Service wuauserv -StartupType Disabled -ErrorAction SilentlyContinue

Set-Content C:\gofuse\setup-done 'ok'
Stop-Transcript
Stop-Computer -Force
