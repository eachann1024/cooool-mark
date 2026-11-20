Unicode true
!include "MUI2.nsh"

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef OUTFILE
  !error "OUTFILE is required"
!endif
!ifndef BINARY
  !error "BINARY is required"
!endif

Name "Cooool Mark"
OutFile "${OUTFILE}"
InstallDir "$PROGRAMFILES64\Cooool Mark"
InstallDirRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "InstallLocation"
RequestExecutionLevel admin
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "Cooool Mark"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileDescription" "Cooool Mark installer"

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetOutPath $INSTDIR
  File /oname=cooool-mark.exe "${BINARY}"
  CreateShortcut "$SMPROGRAMS\Cooool Mark.lnk" "$INSTDIR\cooool-mark.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "DisplayName" "Cooool Mark"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "Publisher" "Cooool Mark"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "NoRepair" 1
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\cooool-mark.exe"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$SMPROGRAMS\Cooool Mark.lnk"
  RMDir "$INSTDIR"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark"
SectionEnd
