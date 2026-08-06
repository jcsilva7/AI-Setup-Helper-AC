---@diagnostic disable: undefined-field
local J = {}

local json = require("json")

--- Convert spinners into Json
---@diagnostic disable-next-line: undefined-doc-name
---@param spinners ac.SetupSpinner[]    Table with the setup data
---@return _ string                     Jsonified data
local function setupSpinnersToJson(spinners)
    local data = {}

    for i, spinner in ipairs(spinners) do
        data[i] = {
            n = spinner.name,
            v = spinner.value,
            min = spinner.minimum,
            max = spinner.maximum,
            s = spinner.step,
            vis = spinner.visible,
            dispMult = spinner.displayMultiplier,
            rOnly = spinner.readOnly,
            unit = spinner.units,
        }

        if spinner.itemValues then
            data[i].items = spinner.itemValues
        end
    end

    return json.encode(data)
end

J.setupSpinnersToJson = setupSpinnersToJson

return J