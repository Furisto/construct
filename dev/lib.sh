#!/bin/bash

delete_agents() {
    echo "Deleting all agents..."
    construct agent list | jq -r '.[].id // empty' | while read -r id; do
        if [ -n "$id" ]; then # Check if id is not empty
            echo "Deleting agent $id"
            construct agent delete "$id"
        fi
    done
    echo "Finished deleting agents."
}

delete_messages() {
    echo "Deleting all messages..."
    construct message list | jq -r '.[].id // empty' | while read -r id; do
        if [ -n "$id" ]; then # Check if id is not empty
            echo "Deleting message $id"
            construct message delete "$id"
        fi
    done
    echo "Finished deleting messages."
}

delete_models() {
    echo "Deleting all models..."
    construct model list | jq -r '.[].id // empty' | while read -r id; do
        if [ -n "$id" ]; then # Check if id is not empty
            echo "Deleting model $id"
            construct model delete "$id"
        fi
    done
    echo "Finished deleting models."
}

delete_model_providers() {
    echo "Deleting all model providers..."
    construct modelprovider list | jq -r '.[].id // empty' | while read -r id; do
        if [ -n "$id" ]; then # Check if id is not empty
            echo "Deleting model provider $id"
            construct modelprovider delete "$id"
        fi
    done
    echo "Finished deleting model providers."
}

delete_tasks() {
    echo "Deleting all tasks..."
    construct task list | jq -r '.[].id // empty' | while read -r id; do
        if [ -n "$id" ]; then 
            echo "Deleting task $id"
            construct task delete "$id"
        fi
    done
    echo "Finished deleting tasks."
}

delete_all_entities() {
    echo "Starting cleanup..."
    delete_messages
    delete_tasks
    delete_agents
    delete_models
    delete_model_providers
    echo "Cleanup finished."
}
