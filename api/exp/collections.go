package api

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/jinzhu/copier"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"

	pb "github.com/nre-learning/syringe/api/exp/generated"
	config "github.com/nre-learning/syringe/config"
)

func (s *SyringeAPIServer) ListCollections(ctx context.Context, filter *pb.CollectionFilter) (*pb.Collections, error) {

	collections := []*pb.Collection{}

	for _, c := range s.Scheduler.Curriculum.Collections {
		collections = append(collections, c)
	}

	return &pb.Collections{
		Collections: collections,
	}, nil
}

func (s *SyringeAPIServer) GetCollection(ctx context.Context, filter *pb.CollectionID) (*pb.Collection, error) {

	collection := &pb.Collection{}
	copier.Copy(&collection, s.Scheduler.Curriculum.Collections[filter.Id])

	for lessonID, lesson := range s.Scheduler.Curriculum.Lessons {
		if lesson.Collection == filter.Id {
			collection.Lessons = append(collection.Lessons, &pb.LessonSummary{
				LessonId:          lessonID,
				LessonDescription: lesson.Description,
				LessonName:        lesson.LessonName,
			})
		}
	}

	return collection, nil
}

func ImportCollections(syringeConfig *config.SyringeConfig) (map[int32]*pb.Collection, error) {

	fileList := []string{}
	collectionDir := fmt.Sprintf("%s/collections", syringeConfig.CurriculumDir)
	log.Debugf("Searching %s for collection definitions", collectionDir)
	err := filepath.Walk(collectionDir, func(path string, f os.FileInfo, err error) error {
		syringeFileLocation := fmt.Sprintf("%s/collection.meta.yaml", path)
		if _, err := os.Stat(syringeFileLocation); err == nil {
			log.Debugf("Found collection definition at: %s", syringeFileLocation)
			fileList = append(fileList, syringeFileLocation)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	retCollections := map[int32]*pb.Collection{}

	for f := range fileList {

		file := fileList[f]

		yamlDef, err := ioutil.ReadFile(file)
		if err != nil {
			log.Errorf("Encountered problem %s", err)
			continue
		}

		var collection pb.Collection
		err = yaml.Unmarshal([]byte(yamlDef), &collection)
		if err != nil {
			log.Errorf("Failed to import %s: %s", file, err)
		}
		collection.CollectionFile = file

		if _, ok := retCollections[collection.Id]; ok {
			log.Errorf("Failed to import %s: Collection ID %d already exists in another collection definition.", file, collection.Id)
			continue
		}

		err = validateCollection(syringeConfig, &collection)
		if err != nil {
			continue
		}
		log.Infof("Successfully imported collection %d: %s", collection.Id, collection.Title)

		retCollections[collection.Id] = &collection
	}

	if len(fileList) == len(retCollections) {
		log.Infof("Imported %d collection definitions.", len(retCollections))
		return retCollections, nil
	} else {
		log.Warnf("Imported %d collection definitions with errors.", len(retCollections))
		return retCollections, errors.New("Not all collection definitions were imported")
	}

}

// validateCollection validates a single collection, returning a simple error if the collection fails
// to validate.
func validateCollection(syringeConfig *config.SyringeConfig, collection *pb.Collection) error {

	// TODO(mierdin): In the future, you should consider putting unique error messages for
	// each violation. This will make this function more testable.
	fail := errors.New("failed to validate collection definition")

	file := collection.CollectionFile

	// Basic validation from protobuf tags
	err := collection.Validate()
	if err != nil {
		log.Errorf("Basic validation failed on %s: %s", file, err)
		return fail
	}

	// More advanced validation
	if syringeConfig.Tier == "prod" {
		if collection.Tier != "prod" {
			log.Errorf("Skipping %s: lower tier than configured", file)
			return fail
		}
	} else if syringeConfig.Tier == "ptr" {
		if collection.Tier != "prod" && collection.Tier != "ptr" {
			log.Errorf("Skipping %s: lower tier than configured", file)
			return fail
		}
	}

	return nil
}
