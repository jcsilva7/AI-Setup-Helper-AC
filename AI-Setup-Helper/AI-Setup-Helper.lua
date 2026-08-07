App_Settings = ac.storage({
    common = false,
    api_key = "",
}, "AISetupHelper.Settings")

-- FIXME: if user leaves setup window cancel the request
-- TODO: improve error handling, missed requests improper api keys

-- Json
local json = require("json")
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

local tempSetupPath = ac.getFolder(ac.FolderID.AppDataLocal)..'/Temp/ai-setup-helper-setup.ini'
local backupSetupPath = ac.getFolder(ac.FolderID.AppDataLocal)..'/Temp/ai-setup-helper-backup.ini'

local hasBackup = false

-- Too much free time
local easter_egg = false
local egg_index = 0
local egg_initial = {
    "You just wait, sunshine, you just wait",
    "I need a white visor. Otherwise, I cannot see anything.",
    "Okay for reference,\n that lap exceeded track limits at turn 19\n so that lap won't count. Don't answer me, thank you.",
    "Kimi, you will have the full information on the steering wheel,\n blue flags, keep pushing...",
    "We think it might be... water.",
}

local egg_last = {
    "Du bist Weltmeister!",
    "Felipe, baby, stay cool",
    "(silence)",
    "Leave me alone, I know what to do.",
    "My goodness, what an idea. Why didn't I think of that?\n Let's add that to the words of wisdom."
}

-- Get random seed and discard first results
math.randomseed(os.time() + math.floor(ac.getSim().currentSessionTime * 1000))
math.random(); math.random(); math.random()

-- The llm has no clue what the values are :|
local WeatherTypeNames = {
    [ac.WeatherType.LightThunderstorm] = "LightThunderstorm",
    [ac.WeatherType.Thunderstorm] = "Thunderstorm",
    [ac.WeatherType.HeavyThunderstorm] = "HeavyThunderstorm",
    [ac.WeatherType.LightDrizzle] = "LightDrizzle",
    [ac.WeatherType.Drizzle] = "Drizzle",
    [ac.WeatherType.HeavyDrizzle] = "HeavyDrizzle",
    [ac.WeatherType.LightRain] = "LightRain",
    [ac.WeatherType.Rain] = "Rain",
    [ac.WeatherType.HeavyRain] = "HeavyRain",
    [ac.WeatherType.LightSnow] = "LightSnow",
    [ac.WeatherType.Snow] = "Snow",
    [ac.WeatherType.HeavySnow] = "HeavySnow",
    [ac.WeatherType.LightSleet] = "LightSleet",
    [ac.WeatherType.Sleet] = "Sleet",
    [ac.WeatherType.HeavySleet] = "HeavySleet",
    [ac.WeatherType.Clear] = "Clear",
    [ac.WeatherType.FewClouds] = "FewClouds",
    [ac.WeatherType.ScatteredClouds] = "ScatteredClouds",
    [ac.WeatherType.BrokenClouds] = "BrokenClouds",
    [ac.WeatherType.OvercastClouds] = "OvercastClouds",
    [ac.WeatherType.Fog] = "Fog",
    [ac.WeatherType.Mist] = "Mist",
    [ac.WeatherType.Smoke] = "Smoke",
    [ac.WeatherType.Haze] = "Haze",
    [ac.WeatherType.Sand] = "Sand",
    [ac.WeatherType.Dust] = "Dust",
    [ac.WeatherType.Squalls] = "Squalls",
    [ac.WeatherType.Tornado] = "Tornado",
    [ac.WeatherType.Hurricane] = "Hurricane",
    [ac.WeatherType.Cold] = "Cold",
    [ac.WeatherType.Hot] = "Hot",
    [ac.WeatherType.Windy] = "Windy",
    [ac.WeatherType.Hail] = "Hail",
}

--- Convert spinners into a Table
---@diagnostic disable-next-line: undefined-doc-name
---@param spinners ac.SetupSpinner[]    Table with the setup data
---@return _ string                     Table data
local function setupSpinnersToTable(spinners)
    local data = {}

    for i, spinner in ipairs(spinners) do
        data[i] = {
            n = spinner.name,
            v = spinner.value,
            min = spinner.minimum,
            max = spinner.maximum,
            s = spinner.step,
            unit = spinner.units,
        }

        if spinner.itemValues then
            data[i].items = spinner.itemValues
        end
    end

    return data
end

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

    local weather = WeatherTypeNames[sim.weatherType] or "Unknown"

    -- Get all available setup data and turn it into a json
    local setup_data = ac.getSetupSpinners()
    local setup_data_table = setupSpinnersToTable(setup_data)

    local data = {
        track_name = ac.getTrackName(),
        layout_name = ac.getTrackLayout(),
        car_name = ac.getCarName(0, true),
        track_temp = track_temp,
        air_temp = air_temp,
        weather = weather,
        -- Default to 20
        laps_fuel = 20,
        setup_data = setup_data_table
    }

    return data
end

local function apply_setup(data) 
    local ok, saved = pcall(json.decode, data)
    if not ok or saved == nil then
        ac.warn("Failed to parse LLM setup response")
        return
    end
    if type(saved) ~= "table" then
        ac.warn("LLM setup response must be a JSON array")
        return
    end

    -- create backup
    ac.saveCurrentSetup(backupSetupPath)
    hasBackup = true

    local spinners = ac.getSetupSpinners()
    local spinnerMap = {}
    for _, spinner in ipairs(spinners) do
        spinnerMap[spinner.name] = spinner
    end

    for _, entry in ipairs(saved) do
        local spinner = spinnerMap[entry.n]
        if spinner and not spinner.readOnly and entry.v ~= nil then
            local v = tonumber(entry.v) or entry.v
            if type(v) == "number" and spinner.min and spinner.max then
                v = math.clamp(v, spinner.min, spinner.max)
            end

            local applied = ac.setSetupSpinnerValue(entry.n, v)
            if not applied then
                ac.warn("Failed to apply setup field: " .. tostring(entry.n))
            end
        else
            ac.warn("Skipping unknown/read-only field: "..tostring(entry.n))
        end
    end
end

local function revert_setup()
    ac.loadSetup(backupSetupPath)
end

local setup_data
-- Main part
function script.windowMain(dt)
    if setup_data == nil then
        setup_data = get_sim_data()
    end
    
    if setup_data == nil then
        return
    end

    -- CONDITIONS --------------------------------------------------
    ui.text('Conditions')
    ui.separator()
    
    if ui.checkbox('Track temp', includeTrackTemp) then includeTrackTemp = not includeTrackTemp end
    if includeTrackTemp then
        setup_data.track_temp = ui.slider('##trackTemp', setup_data.track_temp, 0, 80, 'Track: %.0f°C')
    end
    
    if ui.checkbox('Air temp', includeAirTemp) then includeAirTemp = not includeAirTemp end
    if includeAirTemp then
        setup_data.air_temp = ui.slider('##airTemp', setup_data.air_temp, 0, 60, 'Air: %.0f°C')
    end
    
    if ui.checkbox('Weather', includeWeather) then includeWeather = not includeWeather end
    if not includeWeather then
        setup_data.weather = nil
    end
    
    if ui.checkbox('Fuel', includeFuel) then includeFuel = not includeFuel end
    if includeFuel then
        setup_data["laps_fuel"] = ui.slider('##fuel', setup_data["laps_fuel"], 1, 99, 'Fuel: %.0f laps')
    end
    
    -- FIX -----------------------------------------------------------
    ui.newLine()
    ui.text('Fix')
    ui.separator()
    
    if ui.checkbox('Oversteer', oversteer) then
        oversteer = not oversteer
        understeer = false
    end
    if ui.checkbox('Understeer', understeer) then
        understeer = not understeer
        oversteer = false
    end
    
    -- REQUEST ---------------------------------------------------------
    ui.newLine()
    if ui.button('Request', vec2(-1, 0)) then                
        if request_status == "pending" then
            easter_egg = true
            egg_index = math.random(#egg_initial)
        else
            easter_egg = false
            request_status = "pending"
            request_response = ""
            
            setup_data["oversteer"] = oversteer
            setup_data["understeer"] = understeer

            -- Clear values here, to allow them to appear on the selector
            if not includeAirTemp then
                setup_data.air_temp = nil
            end 

            if not includeWeather then
                setup_data.weather = nil
            end

            local data = json.encode(setup_data)
            
            if type(data) ~= "string" then
                ac.warn("Failed to encode setup data")
                request_status = "failed"
                return
            end

            if App_Settings.common then
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
    end
    
    if request_status == nil then
        ui.text('Awaiting request')
    elseif request_status == 'success' then
        if easter_egg then
            ui.textColored(egg_last[egg_index], rgbm.colors.green)
        else
            ui.textColored('Request acknowledged', rgbm.colors.green)
        end

        if hasBackup then
            ui.sameLine()
            if ui.button('Revert') then
                revert_setup()
                hasBackup = false
                request_status = nil
                request_response = nil
            end
        end
    elseif request_status == 'pending' and easter_egg then
        ui.textColored(egg_initial[egg_index], rgbm.colors.yellow)
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

    if ui.radioButton('Backend', not App_Settings.common) then
        App_Settings.common = false
    end
    ui.sameLine()
    if ui.radioButton('Local', App_Settings.common) then
        App_Settings.common = true
    end

    ui.newLine()
    if App_Settings.common then
        ui.textColored('Requests will be sent using your configuration with your API key.', rgbm.colors.yellow)
    else
        ui.textColored('Requests will be sent to a common service.\nLimited daily requests, shared with all users.', rgbm.colors.yellow)
    end
end