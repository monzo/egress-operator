*** Settings ***
Documentation       Shared keywords for the egress-operator issue acceptance suites.
Library             OperatingSystem
Library             Process
Library             String


*** Variables ***
${ROOT}             ${CURDIR}${/}..${/}..


*** Keywords ***
Run In Repo
    [Documentation]    Run a command from the repository root and return the result.
    ...    Extra named arguments (e.g. env:FOO=bar, timeout=10 min) are passed to Run Process.
    [Arguments]    @{command}    &{config}
    ${result}=    Run Process    @{command}    cwd=${ROOT}    stderr=STDOUT    &{config}
    Log    ${result.stdout}
    RETURN    ${result}

Command Should Succeed
    [Arguments]    @{command}    &{config}
    ${result}=    Run In Repo    @{command}    &{config}
    Should Be Equal As Integers    ${result.rc}    0    msg=${command} failed:\n${result.stdout}
    RETURN    ${result}

Command Should Fail
    [Arguments]    @{command}    &{config}
    ${result}=    Run In Repo    @{command}    &{config}
    Should Not Be Equal As Integers    ${result.rc}    0    msg=${command} unexpectedly succeeded
    RETURN    ${result}

Repo File Should Contain
    [Arguments]    ${path}    ${expected}
    ${content}=    Get File    ${ROOT}${/}${path}
    Should Contain    ${content}    ${expected}

Repo File Should Not Contain
    [Arguments]    ${path}    ${unexpected}
    ${content}=    Get File    ${ROOT}${/}${path}
    Should Not Contain    ${content}    ${unexpected}
