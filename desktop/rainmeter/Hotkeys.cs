using System;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;
using System.Threading;

internal static class CLIProxyAccountsHotkeys
{
    private const int HotkeyId = 17062;
    private const uint Modifiers = 0x4003; // MOD_NOREPEAT | MOD_CONTROL | MOD_ALT
    private static readonly string Identity = @"Local\CLIProxyAccounts.Hotkeys." + WindowsIdentity.GetCurrent().User.Value;
    private static readonly string LogPath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "hotkeys.log");
    private static readonly int Session = Process.GetCurrentProcess().SessionId;
    private delegate bool WindowCallback(IntPtr window, IntPtr parameter);

    [StructLayout(LayoutKind.Sequential)]
    private struct Message
    {
        public IntPtr Window;
        public uint Kind;
        public UIntPtr WParam;
        public IntPtr LParam;
        public uint Time;
        public int X, Y;
        public uint Private;
    }

    [DllImport("user32.dll", SetLastError = true)] private static extern bool RegisterHotKey(IntPtr window, int id, uint modifiers, uint key);
    [DllImport("user32.dll")] private static extern bool UnregisterHotKey(IntPtr window, int id);
    [DllImport("user32.dll", SetLastError = true)] private static extern uint MsgWaitForMultipleObjects(uint count, IntPtr[] handles, bool all, uint milliseconds, uint wakeMask);
    [DllImport("user32.dll")] private static extern bool PeekMessage(out Message message, IntPtr window, uint first, uint last, uint remove);
    [DllImport("user32.dll")] private static extern bool EnumWindows(WindowCallback callback, IntPtr parameter);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetWindowText(IntPtr window, StringBuilder text, int length);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetClassName(IntPtr window, StringBuilder text, int length);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out uint processId);

    private static void Log(string message)
    {
        try
        {
            if (File.Exists(LogPath) && new FileInfo(LogPath).Length > 65536) File.WriteAllText(LogPath, "");
            File.AppendAllText(LogPath, DateTime.UtcNow.ToString("o") + " " + message + Environment.NewLine);
        }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }
    }

    private static bool Acquire(Mutex mutex, int milliseconds)
    {
        try { return mutex.WaitOne(milliseconds); }
        catch (AbandonedMutexException) { return true; }
    }

    private static int Stop()
    {
        using (Mutex mutex = new Mutex(false, Identity))
        {
            if (Acquire(mutex, 0)) { mutex.ReleaseMutex(); return 0; }
            try
            {
                using (EventWaitHandle stop = EventWaitHandle.OpenExisting(Identity + ".Stop")) stop.Set();
            }
            catch (WaitHandleCannotBeOpenedException) { return 2; }
            if (!Acquire(mutex, 10000)) return 2;
            mutex.ReleaseMutex();
            return 0;
        }
    }

    private static bool Active(string rainmeter, string skin)
    {
        bool active = false;
        EnumWindows(delegate(IntPtr window, IntPtr parameter)
        {
            StringBuilder name = new StringBuilder(256);
            GetClassName(window, name, name.Capacity);
            if (name.ToString() != "RainmeterMeterWindow") return true;
            StringBuilder title = new StringBuilder(32768);
            GetWindowText(window, title, title.Capacity);
            if (!String.Equals(Path.GetDirectoryName(title.ToString()), skin, StringComparison.OrdinalIgnoreCase)) return true;
            uint id;
            GetWindowThreadProcessId(window, out id);
            try
            {
                using (Process process = Process.GetProcessById((int)id))
                {
                    if (process.SessionId == Session &&
                        String.Equals(process.MainModule.FileName, rainmeter, StringComparison.OrdinalIgnoreCase)) active = true;
                }
            }
            catch (ArgumentException) { }
            catch (InvalidOperationException) { }
            catch (Win32Exception) { }
            return !active;
        }, IntPtr.Zero);
        return active;
    }

    private static void Command(string rainmeter, string arguments)
    {
        using (Process process = Process.Start(new ProcessStartInfo(rainmeter, arguments)
        {
            WorkingDirectory = Path.GetDirectoryName(rainmeter), UseShellExecute = false, CreateNoWindow = true
        }))
        {
            if (!process.WaitForExit(5000) || process.ExitCode != 0) throw new InvalidOperationException("Rainmeter command failed");
        }
    }

    private static void Toggle(string rainmeter, string skin)
    {
        if (Active(rainmeter, skin))
        {
            Command(rainmeter, "!Toggle \"CLIProxyAccounts\"");
            Log("TOGGLE delivered");
        }
        else
        {
            Command(rainmeter, "!ActivateConfig \"CLIProxyAccounts\" \"Accounts.ini\"");
            Command(rainmeter, "!Show \"CLIProxyAccounts\"");
            Log("RECOVER SHOW delivered");
        }
    }

    [STAThread]
    private static int Main(string[] args)
    {
        if (args.Length == 1 && args[0] == "--stop") return Stop();
        if (args.Length == 1 && args[0] == "--probe")
        {
            if (!RegisterHotKey(IntPtr.Zero, HotkeyId, Modifiers, 0x51)) return Marshal.GetLastWin32Error();
            UnregisterHotKey(IntPtr.Zero, HotkeyId);
            return 0;
        }
        if (args.Length != 2 || !File.Exists(args[0]) || !Directory.Exists(args[1])) return 3;
        using (Mutex mutex = new Mutex(false, Identity))
        {
            if (!Acquire(mutex, 0)) return 0;
            bool registered = false;
            try
            {
                using (EventWaitHandle stop = new EventWaitHandle(false, EventResetMode.ManualReset, Identity + ".Stop"))
                using (EventWaitHandle ready = new EventWaitHandle(false, EventResetMode.ManualReset, Identity + ".Ready"))
                {
                    stop.Reset();
                    registered = RegisterHotKey(IntPtr.Zero, HotkeyId, Modifiers, 0x51);
                    if (!registered) throw new Win32Exception(Marshal.GetLastWin32Error());
                    Log("REGISTERED Ctrl+Alt+Q MOD_NOREPEAT session=" + Session);
                    ready.Set();
                    IntPtr[] handles = { stop.SafeWaitHandle.DangerousGetHandle() };
                    while (true)
                    {
                        uint result = MsgWaitForMultipleObjects(1, handles, false, UInt32.MaxValue, 0x04FF);
                        if (result == 0) break;
                        if (result == UInt32.MaxValue) throw new Win32Exception(Marshal.GetLastWin32Error());
                        Message message;
                        while (PeekMessage(out message, IntPtr.Zero, 0, 0, 1))
                        {
                            if (message.Kind != 0x0312 || message.WParam.ToUInt64() != HotkeyId) continue;
                            Log("KEYPRESS");
                            try { Toggle(Path.GetFullPath(args[0]), Path.GetFullPath(args[1]).TrimEnd('\\')); }
                            catch (Exception) { Log("ERROR dispatch failed"); }
                        }
                    }
                    ready.Reset();
                }
                return 0;
            }
            catch (Exception error)
            {
                Log("ERROR registration or loop failed code=" + (error is Win32Exception ? ((Win32Exception)error).NativeErrorCode : error.HResult));
                return 1;
            }
            finally
            {
                if (registered) UnregisterHotKey(IntPtr.Zero, HotkeyId);
                mutex.ReleaseMutex();
            }
        }
    }
}
