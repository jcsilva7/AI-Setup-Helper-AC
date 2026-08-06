App_Settings = ac.storage({
    localhosting = false,
    api_key = "",
}, "AISetupHelper.Settings")

-- Jsons
local json = require("json")
local c_json = require("custom_json")
-- Requests
local request = require("requests")

local request_status = nil
local request_response = nil

local oversteer = false
local understeer = false

local includeTrackTemp = false
local includeAirTemp = false
local includeWeather = false
local includeFuel = false

--- Get all the sim data required to make the setup
---@return _ table|nil                  The table with all the data
local function get_sim_data()
    if not ac.isSetupAvailableToEdit() then
        return nil
    end

    local sim = ac.getSim()
    if sim == nil then
        return nil
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
        air_temp = air_temp,
        weather = weather,
        setup_data = setup_data_json
    }

    return data
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
    if setup_data == nil then
        return
    end

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
    if not includeWeather then
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
        if request_status == "pending" then
            return
        end

        request_status = "pending"
        request_response = ""
        
        setup_data["oversteer"] = oversteer
        setup_data["understeer"] = understeer
        local data = json.encode(setup_data)
        
        if App_Settings.localhosting then
            request.make_local_request(data, App_Settings.api_key, function(response, success)
                if success then
                    request_status = 'success'
                    request_response = response
                    apply_setup(response)
                else
                    request_status = 'failed'
                    request_response = response
                end
            end)
        else
            -- TODO: backend request
        end
    end
    
    if request_status == nil then
        ui.text('Awaiting request')
    elseif request_status == 'success' then
        ui.textColored('Request acknowledged', rgbm.colors.green)
    elseif request_status == 'pending' then
        ui.textColored('Request pending', rgbm.colors.yellow)
    else
        if request_response ~= nil then
            ui.textColored(request_response, rgbm.colors.red)
        else
            ui.textColored('Request Failed', rgbm.colors.red)
        end
    end
end

function script.windowSettings(dt)
 ui.text('Provider')
    ui.separator()

    ui.text('API Key')
    local newKey, changedKey = ui.inputText('##apiKey', App_Settings.api_key or "", ui.InputTextFlags.Password)
    if changedKey then
        App_Settings.api_key = newKey
    end

    ui.newLine()
    ui.text('Request mode')
    ui.separator()

    if ui.radioButton('Backend', not App_Settings.localhosting) then
        App_Settings.localhosting = false
    end
    ui.sameLine()
    if ui.radioButton('Local', App_Settings.localhosting) then
        App_Settings.localhosting = true
    end

    ui.newLine()
    if App_Settings.localhosting then
        ui.textColored('Requests will be sent to a local server using your API key.', rgbm.colors.yellow)
    else
        ui.textColored('Requests will be sent to a common service.', rgbm.colors.yellow)
    end
end