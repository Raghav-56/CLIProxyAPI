param([switch]$Live, [switch]$InjectKeys)
$ErrorActionPreference='Stop'
$root=Join-Path $env:LOCALAPPDATA 'CLIProxyAccounts'
$temporary=Join-Path ([IO.Path]::GetTempPath()) ('CLIProxyHotkeysTest-'+[Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($temporary)
try {
    $compiler=Join-Path $env:SystemRoot 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    if (!(Test-Path -LiteralPath $compiler)) { $compiler=Join-Path $env:SystemRoot 'Microsoft.NET\Framework\v4.0.30319\csc.exe' }
    $built=Join-Path $temporary 'Hotkeys.exe'
    & $compiler /nologo /target:winexe ('/out:'+$built) (Join-Path $PSScriptRoot 'Hotkeys.cs')
    if ($LASTEXITCODE -ne 0) { throw 'Compilation failed' }
    $mockSource=Join-Path $temporary 'Mock.cs'
    @'
using System;
using System.IO;
public static class MockRainmeter {
    public static int Main(string[] args) {
        if(args.Length==0) {
            var type=typeof(CLIProxyAccountsHotkeys);
            if((uint)type.GetField("Modifiers",System.Reflection.BindingFlags.Static|System.Reflection.BindingFlags.NonPublic).GetRawConstantValue()!=0x4003) return 1;
            type.GetMethod("Toggle",System.Reflection.BindingFlags.Static|System.Reflection.BindingFlags.NonPublic).Invoke(null,new object[]{System.Diagnostics.Process.GetCurrentProcess().MainModule.FileName,AppDomain.CurrentDomain.BaseDirectory});
            var lines=File.ReadAllLines(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"commands.log"));
            return lines.Length==2 && lines[0]=="!ActivateConfig|CLIProxyAccounts|Accounts.ini" && lines[1]=="!Show|CLIProxyAccounts" ? 0 : 1;
        }
        File.AppendAllText(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"commands.log"),String.Join("|",args)+Environment.NewLine);
        return 0;
    }
}
'@ | Set-Content -LiteralPath $mockSource -Encoding UTF8
    $mock=Join-Path $temporary 'Rainmeter.exe'
    & $compiler /nologo /target:winexe /main:MockRainmeter ('/out:'+$mock) $mockSource (Join-Path $PSScriptRoot 'Hotkeys.cs')
    if ($LASTEXITCODE -ne 0) { throw 'Mock compilation failed' }
    $dispatch=Start-Process -FilePath $mock -WindowStyle Hidden -Wait -PassThru
    try { if ($dispatch.ExitCode -ne 0) { throw 'Inactive recovery dispatch failed' } } finally { $dispatch.Dispose() }
    'PASS compile, MOD_NOREPEAT flag, deterministic inactive recovery dispatch through mock executable'
    if (!$Live) { return }

    $config=Get-Content -Raw -LiteralPath (Join-Path $root 'settings.json') | ConvertFrom-Json
    $exe=Join-Path $root 'CLIProxyAccountsHotkeys.exe'
    $rainmeter=Join-Path $config.RainmeterDirectory 'Rainmeter.exe'
    $session=(Get-Process -Id $PID).SessionId
    $meters=@(Get-Process Rainmeter -ErrorAction Stop | Where-Object { $_.SessionId -eq $session -and $_.Path -eq $rainmeter })
    if ($meters.Count -ne 1) { throw 'Run in the interactive Rainmeter session' }
    & (Join-Path $root 'StartHotkeys.ps1')
    $owners=@(Get-Process CLIProxyAccountsHotkeys | Where-Object { $_.SessionId -eq $session -and $_.Path -eq $exe })
    if ($owners.Count -ne 1) { throw 'Expected one hotkey process' }
    & (Join-Path $root 'StartHotkeys.ps1')
    $duplicate=Start-Process -FilePath $exe -ArgumentList @(('"'+$rainmeter+'"'),('"'+$config.SkinDirectory+'"')) -WindowStyle Hidden -PassThru
    if (!$duplicate.WaitForExit(5000) -or $duplicate.ExitCode -ne 0) { throw 'Singleton duplicate failed' }
    $duplicate.Dispose()
    $probe=Start-Process -FilePath $exe -ArgumentList '--probe' -WindowStyle Hidden -Wait -PassThru
    try { if ($probe.ExitCode -ne 1409) { throw "Expected native hotkey contention 1409, got $($probe.ExitCode)" } } finally { $probe.Dispose() }
    & (Join-Path $root 'StartHotkeys.ps1') -Stop
    try {
        $probe=Start-Process -FilePath $exe -ArgumentList '--probe' -WindowStyle Hidden -Wait -PassThru
        try { if ($probe.ExitCode -ne 0) { throw "Ctrl+Alt+Q unavailable after stop: $($probe.ExitCode)" } } finally { $probe.Dispose() }
    } finally { & (Join-Path $root 'StartHotkeys.ps1') }
    'PASS same-session singleton, native RegisterHotKey ownership, cancellation handshake, key release and restart'
    if (!$InjectKeys) { 'SKIP actual keypress and held-key tests; opt in with -Live -InjectKeys'; return }
    Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class CLIProxyHotkeyInputTest {
    delegate bool Callback(IntPtr h, IntPtr p);
    [DllImport("user32.dll")] static extern bool EnumWindows(Callback c, IntPtr p);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetWindowText(IntPtr h,StringBuilder b,int n);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr h,StringBuilder b,int n);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
    public static IntPtr Find(string path) {
        IntPtr found=IntPtr.Zero;
        EnumWindows(delegate(IntPtr h,IntPtr p) {
            var c=new StringBuilder(256);var t=new StringBuilder(32768);
            GetClassName(h,c,c.Capacity);GetWindowText(h,t,t.Capacity);
            if(c.ToString()=="RainmeterMeterWindow" && String.Equals(t.ToString(),path,StringComparison.OrdinalIgnoreCase)) found=h;
            return true;
        },IntPtr.Zero);return found;
    }
    [StructLayout(LayoutKind.Sequential)] struct Keyboard { public ushort Key,Scan; public uint Flags,Time;public UIntPtr Extra; }
    [StructLayout(LayoutKind.Explicit)] struct Data { [FieldOffset(0)]public Keyboard Keyboard;[FieldOffset(0)]public Mouse Mouse; }
    [StructLayout(LayoutKind.Sequential)] struct Mouse { public int X,Y; public uint Data,Flags,Time; public UIntPtr Extra; }
    [StructLayout(LayoutKind.Sequential)] struct Input { public uint Type; public Data Data; }
    [DllImport("user32.dll",SetLastError=true)] static extern uint SendInput(uint n,Input[] input,int size);
    static Input Key(ushort key,bool up) {
        Input input=new Input();input.Type=1;input.Data.Keyboard.Key=key;input.Data.Keyboard.Flags=up?2u:0u;
        return input;
    }
    public static void Press(int repeats) {
        // SendInput inserts one batch without interleaving other keyboard input.
        var input=new System.Collections.Generic.List<Input>();
        input.Add(Key(0x11,false));input.Add(Key(0x12,false));input.Add(Key(0x51,false));
        for(int i=0;i<repeats;i++) input.Add(Key(0x51,false));
        input.Add(Key(0x51,true));input.Add(Key(0x12,true));input.Add(Key(0x11,true));
        if(SendInput((uint)input.Count,input.ToArray(),Marshal.SizeOf(typeof(Input)))!=(uint)input.Count) {
            SendInput(3,new Input[]{Key(0x51,true),Key(0x12,true),Key(0x11,true)},Marshal.SizeOf(typeof(Input)));
            throw new InvalidOperationException("SendInput failed");
        }
    }
}
'@
    $skinPath=Join-Path $config.SkinDirectory 'Accounts.ini'
    $window=[CLIProxyHotkeyInputTest]::Find($skinPath)
    if ($window -eq [IntPtr]::Zero) { throw 'Activate Accounts.ini before the keypress test' }
    $visible=[CLIProxyHotkeyInputTest]::IsWindowVisible($window)
    $originalVisible=$visible
    $foreground=[CLIProxyHotkeyInputTest]::GetForegroundWindow()
    $log=Join-Path $root 'hotkeys.log'
    $countBefore=@(Select-String -LiteralPath $log -Pattern ' KEYPRESS$').Count
    try {
        foreach ($repeat in @(8,0,0,0)) {
            [CLIProxyHotkeyInputTest]::Press($repeat)
            $visible=!$visible
            $deadline=[DateTime]::UtcNow.AddSeconds(5)
            while ([CLIProxyHotkeyInputTest]::IsWindowVisible($window) -ne $visible -and [DateTime]::UtcNow -lt $deadline) { Start-Sleep -Milliseconds 25 }
            if ([CLIProxyHotkeyInputTest]::IsWindowVisible($window) -ne $visible) { throw 'Visibility did not toggle' }
            if ([CLIProxyHotkeyInputTest]::Find($skinPath) -ne $window) { throw 'Active skin was recreated' }
            if ([CLIProxyHotkeyInputTest]::GetForegroundWindow() -ne $foreground) { throw 'Foreground changed during shortcut test' }
        }
        $countAfter=@(Select-String -LiteralPath $log -Pattern ' KEYPRESS$').Count
        if (($countAfter-$countBefore) -ne 4) { throw 'Held-key repeat suppression or dispatch count failed' }
        'PASS actual SendInput Ctrl+Alt+Q four times, eight held-key repeats suppressed, same HWND hide/show/restore, foreground unchanged'
    } finally {
        # Restore the starting visibility even if a test fails midway.
        $current=[CLIProxyHotkeyInputTest]::IsWindowVisible($window)
        if ($current -ne $originalVisible) {
            $bang=if($originalVisible){'!Show'}else{'!Hide'}
            Start-Process -FilePath $rainmeter -ArgumentList ($bang+' "CLIProxyAccounts"') -WindowStyle Hidden -Wait
        }
    }
} finally {
    if ([IO.Path]::GetFullPath($temporary).StartsWith([IO.Path]::GetFullPath([IO.Path]::GetTempPath()),[StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $temporary -Recurse -Force
    }
}
