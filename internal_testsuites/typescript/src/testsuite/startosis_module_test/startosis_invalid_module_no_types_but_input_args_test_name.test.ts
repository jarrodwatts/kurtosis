import {createEnclave} from "../../test_helpers/enclave_setup";
import {DEFAULT_DRY_RUN, EMPTY_EXECUTE_PARAMS, IS_PARTITIONING_ENABLED, JEST_TIMEOUT_MS} from "./shared_constants";
import * as path from "path";
import log from "loglevel";
import {generateScriptOutput} from "../../test_helpers/startosis_helpers";

const INVALID_MODULE_NO_TYPE_BUT_INPUT_ARGS_TEST_NAME = "invalid-module-no-type-input-args";
const INVALID_MODULE_NO_TYPE_BUT_INPUT_ARGS_REL_PATH = "../../../../startosis/invalid-no-type-but-input-args";

jest.setTimeout(JEST_TIMEOUT_MS)

test("Test invalid startosis module no types file but input_args in main", async () => {
    // ------------------------------------- ENGINE SETUP ----------------------------------------------
    const createEnclaveResult = await createEnclave(INVALID_MODULE_NO_TYPE_BUT_INPUT_ARGS_TEST_NAME, IS_PARTITIONING_ENABLED)

    if (createEnclaveResult.isErr()) {
        throw createEnclaveResult.error
    }

    const {enclaveContext, stopEnclaveFunction} = createEnclaveResult.value

    try {
        // ------------------------------------- TEST SETUP ----------------------------------------------
        const moduleRootPath = path.join(__dirname, INVALID_MODULE_NO_TYPE_BUT_INPUT_ARGS_REL_PATH)

        log.info(`Loading module at path '${moduleRootPath}'`)

        const executeStartosisModuleResult = await enclaveContext.executeStartosisModule(moduleRootPath, EMPTY_EXECUTE_PARAMS, DEFAULT_DRY_RUN)

        if (executeStartosisModuleResult.isErr()) {
            log.error(`An error occurred execute startosis module '${moduleRootPath}'`);
            throw executeStartosisModuleResult.error
        }
        const executeStartosisModuleValue = executeStartosisModuleResult.value;

        expect(executeStartosisModuleValue.getInterpretationError()).not.toBeUndefined()
        expect(executeStartosisModuleValue.getInterpretationError()?.getErrorMessage())
            .toContain("Evaluation error: function run missing 1 argument (input_args)")

        expect(executeStartosisModuleValue.getExecutionError()).toBeUndefined()
        expect(executeStartosisModuleValue.getValidationErrors()).toBeUndefined()

        expect(generateScriptOutput(executeStartosisModuleValue.getKurtosisInstructionsList())).toEqual("")
    } finally {
        stopEnclaveFunction()
    }
})
