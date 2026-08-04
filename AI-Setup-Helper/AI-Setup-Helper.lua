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
---@return _ string                  The json with all the data
local function get_sim_data()
    if !ac.isSetupAvailableToEdit() then
        return json.encode({})
    end

    -- Temps and weather
    local track_temp = sim.roadTemperature
    local air_temp = sim.ambientTemperature

    local weather = sim.weatherConditions
    
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
    local setup_data = get_sim_data()
    local request_status = nil

    local oversteer = false
    local understeer = false

    local includeTrackTemp = false
    local includeAirTemp = false
    local includeWeather = false
    local includeFuel = false

    -- CONDITIONS --------------------------------------------------
    ui.text('Conditions')
    ui.separator()
    
    if ui.checkbox('Track temp', includeTrackTemp) then includeTrackTemp = not includeTrackTemp end
    if includeTrackTemp then
        setup_data.track_temp = ui.slider('##trackTemp', setup_data.track_temp, 0, 80, 'Track: %.0f°C')
    else
        setup_data.track_temp = nil
    end
    
    if ui.checkbox('Air temp', includeAirTemp) then includeAirTemp = not includeAirTemp end
    if includeAirTemp then
        setup_data.air_temp = ui.slider('##airTemp', setup_data.air_temp, 0, 60, 'Air: %.0f°C')
    else
        setup_data.air_temp = nil
    end
    
    if ui.checkbox('Weather', includeWeather) then includeWeather = not includeWeather end
    if ~includeWeather then
        setup_data.weather = nil
    end
    
    if ui.checkbox('Fuel', includeFuel) then includeFuel = not includeFuel end
    if includeFuel then
        setup_data["laps_fuel"] = ui.slider('##fuel', 20, 1, 99, 'Fuel: %.0f laps')        
    end
    
    -- FIX -----------------------------------------------------------
    ui.newLine()
    ui.text('Fix')
    ui.separator()
    
    if ui.checkbox('Oversteer', oversteer) then oversteer = not oversteer end
    if ui.checkbox('Understeer', understeer) then understeer = not understeer end
    
    -- REQUEST ---------------------------------------------------------
    ui.newLine()
    if ui.button('Request', vec2(-1, 0)) then
        -- TODO: make request
    end
    
    if request_status == nil then
        ui.text('Awaiting request')
    elseif request_status == 'success' then
        ui.textColored('Request acknowledged', rgbm.colors.green)
    else
        ui.textColored('No response — retry', rgbm.colors.red)
    end
end
