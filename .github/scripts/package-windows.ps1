$ErrorActionPreference = 'Stop'
$version = (Get-Content (Join-Path $PSScriptRoot '../../VERSION') -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid VERSION' }
Set-Location (Join-Path $PSScriptRoot '../..')
$expectedIconCount = @(Get-ChildItem -LiteralPath library -File -Recurse -Filter '*.svg').Count
$release = Join-Path (Get-Location) 'dist/release'
New-Item -ItemType Directory -Force -Path $release | Out-Null
$assetName = "icon-svg-$version-windows-amd64"
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('icon-svg-directory-' + [Guid]::NewGuid())
$package = Join-Path $testRoot 'package'
New-Item -ItemType Directory -Path $package | Out-Null
$exe = Join-Path $package 'icon-svg.exe'
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -ldflags '-s -w' -o $exe .
if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
if ((& $exe -version) -ne $version) { throw 'Unexpected executable version' }

# Verify the complete directory package from a different working directory.
$appDir = Join-Path $testRoot '应用 测试'
$otherDir = Join-Path $testRoot 'unrelated'
New-Item -ItemType Directory -Path $appDir, $otherDir | Out-Null
$standalone = Join-Path $appDir 'icon-svg.exe'
if (Test-Path -LiteralPath 'library/.pending.json') { throw 'Resolve pending library write before packaging' }
Copy-Item library, web -Destination $package -Recurse
$packageLibrary = [IO.Path]::GetFullPath((Join-Path $package 'library'))
Get-ChildItem -LiteralPath $packageLibrary -File -Recurse -Force | Where-Object { $_.Name -eq '.library.lock' -or $_.Name -like '.tmp-*' } | ForEach-Object {
  if (-not $_.FullName.StartsWith($packageLibrary + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unexpected packaging path' }
  Remove-Item -LiteralPath $_.FullName
}
Copy-Item README.md, THIRD_PARTY_NOTICES.txt -Destination $package
New-Item -ItemType Directory -Path (Join-Path $package 'data') | Out-Null
'首次启动会在这里自动生成数据库和管理员凭据。不要将真实数据库或凭据上传到 GitHub。' | Set-Content (Join-Path $package 'data/README.txt') -Encoding utf8
Copy-Item -Path (Join-Path $package '*') -Destination $appDir -Recurse
$savedEnv = @{}
foreach ($key in @('LISTEN_ADDR','OPEN_BROWSER','DATA_DIR','LIBRARY_DIR','SCAN_INTERVAL','WEB_DIR','ADMIN_PASSWORD','ADMIN_USERNAME')) {
  $savedEnv[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
}
try {
  $env:LISTEN_ADDR = '127.0.0.1:18088'
  $env:OPEN_BROWSER = 'false'
  $env:DATA_DIR = ''
  $env:LIBRARY_DIR = ''
  $env:SCAN_INTERVAL = '1s'
  $env:WEB_DIR = ''
  $env:ADMIN_PASSWORD = ''
  $env:ADMIN_USERNAME = 'admin'
  $process = Start-Process -FilePath $standalone -WorkingDirectory $otherDir -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $testRoot 'stdout.log') -RedirectStandardError (Join-Path $testRoot 'stderr.log')
  $ready = $false
  for ($n = 0; $n -lt 60; $n++) {
    if ($process.HasExited) { throw 'Directory package exited before becoming ready' }
    try {
      $health = Invoke-RestMethod -NoProxy 'http://127.0.0.1:18088/healthz'
      if ($health.status -eq 'ok') { $ready = $true; break }
    } catch {}
    Start-Sleep -Milliseconds 500
  }
  if (-not $ready) { throw 'Directory package startup timed out' }
  $homePage = Invoke-WebRequest -NoProxy 'http://127.0.0.1:18088/'
  if (-not $homePage.Content.Contains($expectedIconCount.ToString())) { throw 'External library catalogue is missing' }
  $css = Join-Path $appDir 'web/static/site.css'
  Add-Content -LiteralPath $css -Value '/* external-directory-probe */'
  $asset = Invoke-WebRequest -NoProxy 'http://127.0.0.1:18088/static/site.css'
  if (-not $asset.RawContent.Contains('external-directory-probe')) { throw 'Server did not read external web files' }
  $metaPath = Join-Path $appDir 'library/feng/meta.json'
  $meta = Get-Content -LiteralPath $metaPath -Raw | ConvertFrom-Json
  $meta.title = 'external-library-probe'
  $meta | ConvertTo-Json -Depth 64 | Set-Content -LiteralPath $metaPath -Encoding utf8
  $scanned = $false
  for ($n = 0; $n -lt 20; $n++) {
    Start-Sleep -Milliseconds 500
    $search = Invoke-WebRequest -NoProxy 'http://127.0.0.1:18088/?q=external-library-probe'
    if ($search.Content.Contains('external-library-probe') -and $search.Content.Contains('/detail/')) { $scanned = $true; break }
  }
  if (-not $scanned) { throw 'Periodic scan did not index external library metadata' }
  $db = Join-Path $appDir 'data/runtime.sqlite3'
  $credentials = Join-Path $appDir 'data/initial-admin.txt'
  if (-not (Test-Path -LiteralPath $credentials)) { throw 'Initial credentials were not created beside EXE' }
  if (Test-Path (Join-Path $otherDir 'data')) { throw 'Data was created in the working directory' }
  $check = @'
import sqlite3,sys
c=sqlite3.connect(sys.argv[1])
for table,count in {'audit':0,'sessions':0,'attempts':0,'downloads':0}.items():
    assert c.execute('SELECT COUNT(*) FROM '+table).fetchone()[0]==count,table
assert c.execute("SELECT COUNT(*) FROM sqlite_master WHERE name IN ('brands','groups','icons','categories')").fetchone()[0]==0
assert c.execute('PRAGMA integrity_check').fetchone()[0]=='ok'
print('Directory package verified: external library/web, independent data, empty logs/counters.')
'@
  python -c $check $db
  if ($LASTEXITCODE -ne 0) { throw 'Initial database verification failed' }
} finally {
  if ($process -and -not $process.HasExited) { Stop-Process -Id $process.Id -Force }
  foreach ($key in $savedEnv.Keys) { [Environment]::SetEnvironmentVariable($key, $savedEnv[$key], 'Process') }
  # Keep failed startup logs in runner temp for diagnosis; none are packaged.
}

Compress-Archive -Path (Join-Path $package '*') -DestinationPath (Join-Path $release "$assetName.zip") -Force
python -c "import zipfile,sys; z=zipfile.ZipFile(sys.argv[1]); names=z.namelist(); assert 'icon-svg.exe' in names; assert 'web/templates/home.html' in names; assert 'data/README.txt' in names; assert len([x for x in names if x.startswith('library/') and x.endswith('.svg')])==int(sys.argv[2]); assert not any(x.endswith(('.sqlite3','initial-admin.txt')) for x in names)" (Join-Path $release "$assetName.zip") $expectedIconCount
if ($LASTEXITCODE -ne 0) { throw 'Directory ZIP contents verification failed' }
$hashes = Get-ChildItem $release -File | Where-Object Extension -eq '.zip' | Sort-Object Name | ForEach-Object {
  (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $_.Name
}
$hashes | Set-Content (Join-Path $release 'SHA256SUMS.txt') -Encoding ascii
Write-Output "Packaged icon-svg $version for Windows amd64."
