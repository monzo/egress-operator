*** Settings ***
Documentation       Issue #50: `make test` must work without manually installing setup-envtest or
...                 exporting KUBEBUILDER_ASSETS, and the expected coverage must be documented.
...                 https://github.com/monzo/egress-operator/issues/50
Resource            resources/common.resource
Test Tags           issue-50


*** Test Cases ***
Go Test Without Envtest Assets Fails With The Documented Error
    [Documentation]    Reproduces the original report: plain `go test` has no control plane binaries.
    [Tags]    slow
    ${result}=    Command Should Fail    go    test    ./controllers/    -count\=1
    ...    env:KUBEBUILDER_ASSETS=    timeout=5 min
    Should Contain    ${result.stdout}    unable to start the controlplane

Make Test Works Without KUBEBUILDER_ASSETS
    [Tags]    slow
    ${result}=    Command Should Succeed    make    test    env:KUBEBUILDER_ASSETS=    timeout=15 min
    Should Match Regexp    ${result.stdout}    ok\\s+github.com/monzo/egress-operator/controllers\\s.*coverage: \\d+\\.\\d% of statements
    File Should Exist    ${ROOT}${/}cover.out

Controllers Coverage Meets The Documented Minimum
    [Tags]    slow
    ${result}=    Command Should Succeed    make    coverage-check
    Should Contain    ${result.stdout}    controllers coverage:

README Documents Test Prerequisites And Expected Coverage
    Repo File Should Contain    README.md    KUBEBUILDER_ASSETS
    Repo File Should Contain    README.md    setup-envtest
    Repo File Should Contain    README.md    make coverage-check
    Repo File Should Contain    README.md    /usr/local/kubebuilder/bin/etcd
