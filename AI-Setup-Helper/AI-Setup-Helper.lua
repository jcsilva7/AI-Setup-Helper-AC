local sim = ac.getSim()

-- UI functions
local UI = require("ui")
-- Jsons
local json = require("json")
local c_json = require("custom_json")

-- Dotenv load
-- local dotenv = require("dotenv")
-- dotenv.load()

--- Get all the sim data required to make the setup
---@param include_temps boolean      If the track and air temp should be considered
---@param include_weather boolean    If the weather should be considered
---@return _ string                  The json with all the data
local function get_sim_data(include_temps, include_weather)
    if !ac.isSetupAvailableToEdit() then
        return json.encode({})
    end

    local track_temp = nil
    local air_temp = nil
    if include_temps then
        track_temp = sim.roadTemperature
        air_temp = sim.ambientTemperature
    end

    local weather = nil
    if include_weather then
        weather = sim.weatherConditions
    end
    
    -- Get all available setup data and turn it into a json
    local setup_data = ac.getSetupSpinners()
    local setup_data_json = c_json.setupSpinnersToJson(setup_data)

    local data = {
        track_name = ac.getTrackName(),
        layout_name = ac.getTrackLayout(),
        car_name = ac.getCarName(0, true),
        track_temp = track_temp,
        weather = weather,
        setup_data = setup_data_json
    }

    return json.encode(data)
end

local function apply_setup(data) 
    local saved = json.decode(data)
    local spinners = ac.getSetupSpinners()

    -- Index current spinners by their internal name
    local spinnerMap = {}

    for _, spinner in ipairs(spinners) do
        spinnerMap[spinner.name] = spinner
    end

    -- Apply saved values
    for _, entry in ipairs(saved) do
        local spinner = spinnerMap[entry.n]
        if spinner and not spinner.readOnly then
            spinner.value = entry.v
        end
    end
end

-- Main part
function script.windowMain(dt)
    -- TODO: Fuel
    -- TODO: UI
end
