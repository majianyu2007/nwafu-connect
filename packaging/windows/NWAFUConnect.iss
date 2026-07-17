#ifndef SourceDir
#define SourceDir "dist"
#endif
#ifndef OutputDir
#define OutputDir "dist"
#endif
#ifndef Version
#define Version "1.4.0"
#endif

[Setup]
AppId={{B165221A-0C04-42CC-92A5-D78286E7B06D}
AppName=NWAFU Connect
AppVersion={#Version}
AppPublisher=NWAFU Connect contributors
DefaultDirName={localappdata}\Programs\NWAFU Connect
DefaultGroupName=NWAFU Connect
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#OutputDir}
OutputBaseFilename=NWAFU-Connect-{#Version}-windows-amd64-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\nwafu-connect-desktop.exe
SetupIconFile={#SourceDir}\icon.ico

[Files]
Source: "{#SourceDir}\nwafu-connect-desktop.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\nwafu-connect.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\nwafu-connect-proxy.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\icon.ico"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\NWAFU Connect"; Filename: "{app}\nwafu-connect-desktop.exe"; IconFilename: "{app}\icon.ico"

[Run]
Filename: "{app}\nwafu-connect-desktop.exe"; Description: "启动 NWAFU Connect"; Flags: nowait postinstall skipifsilent
